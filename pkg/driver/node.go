// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// Copyright (C) 2020 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	guuid "github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	mountutils "k8s.io/mount-utils"

	"github.com/lightbitslabs/los-csi/pkg/driver/backend"
	"github.com/lightbitslabs/los-csi/pkg/lb"
	"github.com/lightbitslabs/los-csi/pkg/util/endpoint"
	"github.com/lightbitslabs/los-csi/pkg/util/wait"
)

// lbVolEligible() allows to rule out impossible scenarios early on. it
// checks if the volume exists on the LightOS cluster and is fully accessible
// by this host configuration-wise and in terms of target-side availability.
// all of this might change by the time we actually try to connect/mount, of
// course, but usually only for the worse, not for the better.
func (d *Driver) lbVolEligible(
	ctx context.Context, log *logrus.Entry, clnt lb.Client, vid lbResourceID,
) error {
	vol, err := clnt.GetVolume(ctx, vid.uuid, vid.projName)
	if err != nil {
		if isStatusNotFound(err) {
			return mkEnoent("volume '%s' doesn't exist", vid)
		}
		return d.mungeLBErr(log, err, "failed to get volume '%s' from LB", vid)
	}

	if !vol.IsUsable() {
		log.Warnf("volume is inaccessible, state: %s, protection: %s",
			vol.State, vol.Protection)
		return mkEagain("volume '%s' is temporarily inaccessible", vid)
	}

	accessible := false
	// ACLs are normally VERY short, no point in showboating...
	for _, ace := range vol.ACL {
		if ace == d.hostNQN {
			accessible = true
			break
		}
	}
	if !accessible {
		log.Warnf("volume is inaccessible from '%s', HostNQN: '%s', ACL: %#q",
			d.nodeID, d.hostNQN, vol.ACL)
		return mkPrecond("volume '%s' is inaccessible from node '%s'", vid, d.nodeID)
	}

	st := d.be.LBVolEligible(ctx, vol)
	return st.Err()
}

func (d *Driver) queryLBforTargetEnv(
	ctx context.Context, log *logrus.Entry, clnt lb.Client, vid lbResourceID,
) (*backend.TargetEnv, error) {
	ci, err := clnt.GetClusterInfo(ctx)
	if err != nil {
		return nil, d.mungeLBErr(log, err, "failed to get info from LB cluster at '%s'",
			vid.mgmtEPs[0])
	}

	res := &backend.TargetEnv{
		SubsysNQN: ci.SubsysNQN,
	}
	res.DiscoveryEPs, err = endpoint.ParseSliceIPv4(ci.DiscoveryEndpoints)
	if err != nil {
		return nil, d.mungeLBErr(log, err,
			"got invalid discovery service endpoint from LB cluster at '%s'",
			vid.mgmtEPs[0])
	}
	res.NvmeEPs, err = endpoint.ParseSliceIPv4(ci.NvmeEndpoints)
	if err != nil {
		return nil, d.mungeLBErr(log, err,
			"got invalid NVMe endpoint from LB cluster at '%s'", vid.mgmtEPs[0])
	}

	return res, nil
}

func (d *Driver) getDeviceUUID(device string) (string, error) {
	devUUID, err := ioutil.ReadFile(filepath.Join("/sys/block", device, "wwid"))
	if err != nil {
		d.log.Debugf("failed to read wwid from dev: %s err: %s", device, err)
		return "", err
	}

	devUUIDstr := strings.TrimSuffix(string(devUUID), "\n")
	// LightOS always exposes uuid, so if we don't see a uuid identifier we
	// can safely return a mismatch
	if strings.Contains(devUUIDstr, "uuid") {
		return strings.TrimPrefix(devUUIDstr, "uuid."), nil
	}
	return "", nil
}

func (d *Driver) GetDevPathByUUID(uuid guuid.UUID) (string, error) {
	// first try to get by-id device symlink, but ignore the error as older
	// kernels might not have that yet.
	linkPath := filepath.Join("/dev/disk/by-id", "nvme-uuid."+uuid.String())
	devicePath, err := filepath.EvalSymlinks(linkPath)
	if err == nil {
		return filepath.Abs(devicePath)
	}

	// regex for all nvme devices
	devices, err := filepath.Glob(filepath.Join("/dev", "nvme[0-9]*n[0-9]*"))
	if err != nil {
		return "", err
	}

	// filters:
	// 1. remove partitions - we don't care about those (/dev/nvmeXnYpZ)
	// 2. hidden devices: in older kernels devices in the form /dev/nvmeXcYnZ
	effDevices := []string{}
	for _, d := range devices {
		if !strings.Contains(d, "c") && !strings.Contains(d, "p") {
			effDevices = append(effDevices, d)
		}
	}

	// iterate over devices and try to find a matching uuid
	for _, dev := range effDevices {
		devUUID, err := d.getDeviceUUID(filepath.Base(dev))
		if err != nil {
			// ignoring errors to get device UUID
			// because apparently some unexpected device
			// identifications were found in the wild
			// between kernel backport quirks and old
			// devices that expose outdated identifications.
			return "", nil
		}
		if devUUID == uuid.String() {
			// found a match!
			return dev, nil
		}
	}
	d.log.Debugf("didn't find uuid: %s in effDevices: %s", uuid.String(), effDevices)

	// nothing showed up...
	return "", nil
}

func (d *Driver) getDevicePath(uuid guuid.UUID) (string, error) {
	devPath := ""
	var err error = nil
	err = wait.WithRetries(30, 100*time.Millisecond, func() (bool, error) {
		devPath, err = d.GetDevPathByUUID(uuid)
		return devPath != "", err
	})
	if err != nil || devPath == "" {
		return "", mkEExec("no local block device for volume %s", uuid)
	}
	d.log.Debugf("block device representing volume is: '%s'", devPath)
	return devPath, nil
}

// ConstructMountOptions returns only unique mount options in slice.
func ConstructMountOptions(mountOptions []string, volCap *csi.VolumeCapability) []string {
	if m := volCap.GetMount(); m != nil {
		hasOption := func(options []string, opt string) bool {
			for _, o := range options {
				if o == opt {
					return true
				}
			}

			return false
		}
		for _, f := range m.MountFlags {
			if !hasOption(mountOptions, f) {
				mountOptions = append(mountOptions, f)
			}
		}
	}

	return mountOptions
}

// IsVolumeReadOnly checks the access mode in Volume Capability and decides
// if volume is readonly or not.
func IsVolumeReadOnly(capability *csi.VolumeCapability) bool {
	accMode := capability.GetAccessMode().GetMode()
	ro := false
	if accMode == csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY ||
		accMode == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
		ro = true
	}
	return ro
}

// CSI Node service: ---------------------------------------------------------

// NodeStageVolume obtains the necessary NVMe-oF target(s) endpoints from the
// LB mgmt API server(s) specified in `NodeStageVolumeRequest.volume_id`, then
// establishes data plane connections to them to attach the volume to the node.
// it will also format the volume with a specified FS, if necessary, or try to
// recover the FS (q.v. fsck) if an existing one is detected.
func (d *Driver) NodeStageVolume(
	ctx context.Context, req *csi.NodeStageVolumeRequest,
) (*csi.NodeStageVolumeResponse, error) {
	if req.StagingTargetPath == "" {
		return nil, mkEinvalMissing("staging_target_path")
	}
	if err := d.validateVolumeCapability(req.VolumeCapability); err != nil {
		return nil, err
	}

	vid, err := ParseCSIResourceID(req.VolumeId)
	if err != nil {
		return nil, mkEinval("volume_id", err.Error())
	}

	log := d.log.WithFields(logrus.Fields{
		"op":       "NodeStageVolume",
		"mgmt-ep":  vid.mgmtEPs,
		"vol-uuid": vid.uuid,
		"project":  vid.projName,
		"scheme":   vid.scheme,
	})

	ctx = d.cloneCtxWithCreds(ctx, req.Secrets)
	clnt, err := d.GetLBClient(ctx, vid.mgmtEPs, vid.scheme)
	if err != nil {
		return nil, err
	}
	defer d.PutLBClient(clnt)

	// remote/global sanity check: - - - - - - - - - - - - - - - - - - - -

	err = d.lbVolEligible(ctx, log, clnt, vid)
	if err != nil {
		return nil, err
	}

	d.bdl.Lock() // TODO: break up into per-volume+per-target locks!
	defer d.bdl.Unlock()

	// node local sanity checks: - - - - - - - - - - - - - - - - - - - - -

	// ALLEGEDLY, StagingTargetPath is supposed to exist by this point,
	// so i don't think a plugin is supposed to be creating it (more likely
	// an indication of some misconfiguration or plugin looking at a wrong
	// path, or some such). and i THINK i can see `kubelet` creating it
	// before calling the plugins...
	tgtPath := req.StagingTargetPath
	notMnt, err := mountutils.IsNotMountPoint(d.mounter, tgtPath)
	if os.IsNotExist(err) {
		return nil, mkEinvalf("staging_target_path",
			"'%s' doesn't exist", tgtPath)
	} else if err != nil {
		return nil, mkEExec("can't examine staging path: %s", err)
	}
	// don't you not like double negatives?
	if !notMnt {
		dev, _, err := mountutils.GetDeviceNameFromMount(d.mounter, tgtPath)
		if err != nil {
			log.Debugf("failed to find what's mounted at '%s': %s", tgtPath, err)
		}
		log.Debugf("'%s' is already mounted at '%s'", dev, tgtPath)

		// TODO: check that the FS didn't auto-remount as RO due to
		// disconnect/error?

		// TODO: NO, not really, now we need to check that it's indeed
		// the right volume (use client.GetNGUIDByDevPath() like
		// NodeUnstageVolume() for this reverse mapping then compare to
		// VolumeID), with the right FS (check against caps) and mounted
		// with the right flags (ditto)! i.e. that presumably k8s is
		// just calling us again for a retry of a request that may have
		// timed out, or some such...
		return &csi.NodeStageVolumeResponse{}, nil
	}

	// get remote NVMe-oF targets info from the LB cluster:  - - - - - - -

	tgtEnv, err := d.queryLBforTargetEnv(ctx, log, clnt, vid)
	if err != nil {
		return nil, err
	}

	// let backend connect and produce block device: - - - - - - - - - - -

	if st := d.be.Attach(ctx, tgtEnv, vid.uuid); st != nil {
		return nil, st.Err()
	}

	devPath, err := d.getDevicePath(vid.uuid)
	if err != nil {
		return nil, err
	}

	// turn block dev into what CO wanted: - - - - - - - - - - - - - - - -

	if req.VolumeCapability.GetBlock() != nil {
		// block volume - we are done for now.
		return &csi.NodeStageVolumeResponse{}, nil
	}

	mntCap := req.VolumeCapability.GetMount()
	if mntCap == nil {
		return nil, mkEinvalMissing("volume_capability.mount")
	}
	wantFSType := mntCap.FsType
	fsType, err := d.mounter.GetDiskFormat(devPath)
	if err != nil {
		return nil, mkEExec("error examining format of volume %q. devPath: %q. error: %v", vid.uuid, devPath, err)
	}
	if fsType != "" {
		if fsType != wantFSType && wantFSType != "" {
			return nil, mkEbadOp("mismatch", vid.uuid.String(),
				"requested FS '%s', but volume already contains FS '%s'",
				wantFSType, fsType)
		}
		wantFSType = fsType
	} else if wantFSType == "" {
		wantFSType = d.defaultFS
	}

	// TODO: actually, derive them from `mntCap.MountFlags` and `wantRO`
	// above, if any.
	mntOpts := []string{}
	mntOpts = ConstructMountOptions(mntOpts, req.GetVolumeCapability())
	ro := IsVolumeReadOnly(req.GetVolumeCapability())

	if ro {
		mntOpts = append(mntOpts, "ro")
	}

	if wantFSType == "xfs" {
		mntOpts = append(mntOpts, "nouuid")
	}

	// theoretically, FormatAndMount() will only "Format" if the volume has
	// no FS yet, but... additionally, "safe" though it claims to be, it is
	// NOT a strict NOP in cases where it will eventually turn out to be a
	// mismatch in FS type, so only try it if there's a reasonable chance
	// of success (i.e. either no FS or at least the same FS type as the
	// required one)...
	err = d.mounter.FormatAndMount(devPath, tgtPath, wantFSType, mntOpts)
	if err != nil {
		return nil, mkEExec("format/mount failed: '%s'", err.Error())
	}

	// In case the volume came from a snapshot and happens to also be
	// larger in capacity, we want to update the fs size accordingly, we
	// don't care really if any actual resize happened...
	resizer := mountutils.NewResizeFs(d.mounter.Exec)
	_, err = resizer.Resize(devPath, tgtPath)
	if err != nil {
		return nil, mkEExec("error when resizing device %s after mount: %v", vid.uuid, err)
	}

	log.Debugf("OK, volume '%s' mounted from '%s' to '%s' "+
		"with '%s' FS", vid.uuid, devPath, tgtPath, wantFSType)
	return &csi.NodeStageVolumeResponse{}, nil
}

func (d *Driver) NodeUnstageVolume(
	ctx context.Context, req *csi.NodeUnstageVolumeRequest,
) (*csi.NodeUnstageVolumeResponse, error) {
	if req.StagingTargetPath == "" {
		return nil, mkEinvalMissing("staging_target_path")
	}
	tgtPath := req.StagingTargetPath

	vid, err := ParseCSIResourceID(req.VolumeId)
	if err != nil {
		return nil, mkEinval("volume_id", err.Error())
	}

	d.bdl.Lock() // TODO: break up into per-volume+per-target locks!
	defer d.bdl.Unlock()

	// TODO: check that staging target path indeed corresponds to the NVMe
	// device, using client.GetNGUIDByDevPath(). same check is needed in
	// NodeStageVolume() to support idempotent retries... except here, due
	// to the idempotency requirement it's weakened: it can only weed out
	// odd collisions between live mount paths, but NOT, say, missing NVMe
	// devices or even mountpoints - that might be a side-effect of k8s
	// retrying the call...

	notMnt, err := mountutils.IsNotMountPoint(d.mounter, tgtPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, mkEExec("can't examine staging path: %s", err)
	}
	if !notMnt {
		err = d.mounter.Unmount(tgtPath)
		if err != nil {
			return nil, mkEExec("failed to unmount '%s': %s", tgtPath, err)
		}
	}

	if st := d.be.Detach(ctx, vid.uuid); st != nil {
		return nil, st.Err()
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (d *Driver) nodePublishVolumeForBlock(
	vid lbResourceID, log *logrus.Entry, req *csi.NodePublishVolumeRequest,
	mountOptions []string,
) (*csi.NodePublishVolumeResponse, error) {
	target := req.GetTargetPath()
	source, err := d.getDevicePath(vid.uuid)
	if err != nil {
		return &csi.NodePublishVolumeResponse{}, mkEExec("can't examine device path: %s", err)
	}

	// TODO add idempotency support

	// Create the mount point as a file since bind mount device node requires it to be a file
	log.Debugf("Creating target file %s", target)
	err = MakeFile(target)
	if err != nil {
		if removeErr := os.Remove(target); removeErr != nil {
			return &csi.NodePublishVolumeResponse{},
				status.Errorf(codes.Internal, "Could not remove mount target %q: %v", target, removeErr)
		}
		return &csi.NodePublishVolumeResponse{},
			status.Errorf(codes.Internal, "Could not create file %q: %v", target, err)
	}

	log.Debugf("bind-mount %s at %s", source, target)
	if err := d.mounter.Mount(source, target, "", mountOptions); err != nil {
		if removeErr := os.Remove(target); removeErr != nil {
			return &csi.NodePublishVolumeResponse{},
				status.Errorf(codes.Internal, "Could not remove mount target %q: %v", target, removeErr)
		}
		return &csi.NodePublishVolumeResponse{},
			status.Errorf(codes.Internal, "Could not mount %q at %q: %v", source, target, err)
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func MakeFile(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE, os.FileMode(0644))
	if err != nil {
		if !os.IsExist(err) {
			return err
		}
	}
	if err = f.Close(); err != nil {
		return err
	}
	return nil
}

func getDeviceNameFromMount(ctx context.Context, tgtPath string) (string, error) {
	info, err := mountutils.ListProcMounts(tgtPath)
	if err != nil {
		return "", err
	}

	for _, m := range info {
		if tgtPath == m.Path {
			return m.Device, nil
		}
	}

	return "", mkEinvalf("Failed to find device for tgtPath", "'%s'", tgtPath)
}

func (d *Driver) nodePublishVolumeForFileSystem(
	vid lbResourceID, log *logrus.Entry, req *csi.NodePublishVolumeRequest,
	mountOptions []string,
) (*csi.NodePublishVolumeResponse, error) {
	stagingPath := req.StagingTargetPath
	tgtPath := req.TargetPath
	if err := os.MkdirAll(tgtPath, 0750); err != nil {
		return nil, mkEinvalf("Failed to create target_path", "'%s'", tgtPath)
	}

	err := d.mounter.Mount(stagingPath, tgtPath, "", mountOptions)
	if err != nil {
		return nil, mkEExec("failed to bind mount: %s", err)
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

// TODO: see the wonderful mandated error codes table covering NodePublishVolume
// return value semantics... (NOT currently implemented that way!)
func (d *Driver) NodePublishVolume(
	ctx context.Context, req *csi.NodePublishVolumeRequest,
) (*csi.NodePublishVolumeResponse, error) {
	vid, err := ParseCSIResourceID(req.VolumeId)
	if err != nil {
		return nil, mkEinval("volume_id", err.Error())
	}

	logFields := logrus.Fields{
		"op": "NodePublishVolume",
	}
	if req.VolumeContext != nil {
		if podName, ok := req.VolumeContext["csi.storage.k8s.io/pod.name"]; ok {
			logFields["pod-name"] = podName
		}
		if podNS, ok := req.VolumeContext["csi.storage.k8s.io/pod.namespace"]; ok {
			logFields["pod-ns"] = podNS
		}
		if podUID, ok := req.VolumeContext["csi.storage.k8s.io/pod.uid"]; ok {
			logFields["pod-uid"] = podUID
		}
	}

	logFields["mgmt-ep"] = vid.mgmtEPs
	logFields["vol-uuid"] = vid.uuid
	logFields["project"] = vid.projName
	log := d.log.WithFields(logFields)

	if req.StagingTargetPath == "" {
		return nil, mkEbadOp("ordering", "staging_target_path",
			"volume must be staged before publishing")
	}
	if req.TargetPath == "" {
		return nil, mkEinvalMissing("target_path")
	}
	if err := d.validateVolumeCapability(req.VolumeCapability); err != nil {
		return nil, err
	}
	volCap := req.GetVolumeCapability()
	// TODO: check capabilities, flags, mode, etc.

	mountOptions := []string{"bind"}
	if req.GetReadonly() {
		log.Debugf("Publish as ReadOnly")
		mountOptions = append(mountOptions, "ro")
	}

	d.bdl.Lock() // TODO: break up into per-volume+per-target locks!
	defer d.bdl.Unlock()

	// for idempotency - start in reverse order:
	if _, err := os.Stat(req.TargetPath); err == nil {
		notMnt, err := mountutils.IsNotMountPoint(d.mounter, req.TargetPath)
		if os.IsNotExist(err) {
			return nil, mkEinvalf("target_path", "'%s' doesn't exist", req.TargetPath)
		} else if err != nil {
			return nil, mkEExec("can't examine target path: %s", err)
		}
		if !notMnt {
			dev, err := getDeviceNameFromMount(ctx, req.TargetPath)
			if err != nil {
				log.Debugf("failed to find what's mounted at '%s': %s", req.TargetPath, err)
			}

			// TODO: WARNING: if for some reason `nvme disconnect` (or its moral
			// equivalent) affecting the target in question happened at some point
			// before this, the FS remains *MOUNTED* there, just in a broken state,
			// obviously (EIO on most accesses), so this code will just say
			// "'/dev/nvme0n1' is already mounted at '<blah-staging-path>'", but
			// will NOT actually remount anything (as it probably should have!)
			// and return success instead, keeping the volume in a totally
			// inaccessible state, and repeating the same silliness on retries! */

			log.Debugf("'%s' is already mounted at '%s'", dev, req.TargetPath)

			return &csi.NodePublishVolumeResponse{}, nil
		}
	}

	// if not yet - do it manually:
	switch volCap.GetAccessType().(type) {
	case *csi.VolumeCapability_Block:
		return d.nodePublishVolumeForBlock(vid, log, req, mountOptions)
	case *csi.VolumeCapability_Mount:
		return d.nodePublishVolumeForFileSystem(vid, log, req, mountOptions)
	default:
		return nil, mkEinval("volume capability", "unknown volume capability")
	}
}

func (d *Driver) NodeUnpublishVolume(
	ctx context.Context, req *csi.NodeUnpublishVolumeRequest,
) (*csi.NodeUnpublishVolumeResponse, error) {
	if req.TargetPath == "" {
		return nil, mkEinvalMissing("target_path")
	}

	_, err := ParseCSIResourceID(req.VolumeId)
	if err != nil {
		return nil, mkEinval("volume_id", err.Error())
	}

	d.bdl.Lock() // TODO: break up into per-volume+per-target locks!
	defer d.bdl.Unlock()

	tgtPath := req.TargetPath
	notMnt, err := mountutils.IsNotMountPoint(d.mounter, tgtPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, mkEExec("can't examine staging path: %s", err)
	}
	if !notMnt {
		err = d.mounter.Unmount(tgtPath)
		if err != nil {
			return nil, mkEExec("failed to unmount '%s': %s", tgtPath, err)
		}
	}

	if err = os.RemoveAll(tgtPath); err != nil {
		return nil, mkEExec("failed to remove '%s': %s", tgtPath, err)
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (d *Driver) NodeGetCapabilities(
	ctx context.Context, req *csi.NodeGetCapabilitiesRequest,
) (*csi.NodeGetCapabilitiesResponse, error) {
	capabilities := []*csi.NodeServiceCapability{
		{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
				},
			},
		},
		{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					// TODO: this might depend on the backend used - some
					// backends might not support resize out of the box...
					Type: csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
				},
			},
		},
		{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
				},
			},
		},
	}

	return &csi.NodeGetCapabilitiesResponse{Capabilities: capabilities}, nil
}

func (d *Driver) NodeGetInfo(
	ctx context.Context, req *csi.NodeGetInfoRequest,
) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId: d.nodeID,
	}, nil
}

func (d *Driver) NodeGetVolumeStats(
	ctx context.Context, req *csi.NodeGetVolumeStatsRequest,
) (*csi.NodeGetVolumeStatsResponse, error) {
	vid, err := ParseCSIResourceID(req.VolumeId)
	if err != nil {
		return nil, mkEinval("volume_id", err.Error())
	}

	log := d.log.WithFields(logrus.Fields{
		"op":       "NodeGetVolumeStats",
		"mgmt-ep":  vid.mgmtEPs,
		"vol-uuid": vid.uuid,
		"vol-path": req.VolumePath,
		"project":  vid.projName,
	})

	targetPath := req.GetVolumePath()
	log.Infof("called for volume %q in project %q, targetPath: %q", vid.uuid, vid.projName, targetPath)
	if targetPath == "" {
		err = fmt.Errorf("targetpath %v is empty", targetPath)

		return nil, mkEinval("targetPath", err.Error())
	}

	stat, err := os.Stat(targetPath)
	if err != nil {
		return nil, mkEinval("targetPath", fmt.Sprintf("failed to get stat for targetpath %q: %v", targetPath, err))
	}

	if stat.Mode().IsDir() {
		return filesystemNodeGetVolumeStats(ctx, log, targetPath)
	} else if (stat.Mode() & os.ModeDevice) == os.ModeDevice {
		return blockNodeGetVolumeStats(ctx, log, targetPath)
	}

	return nil, mkEinval("targetPath", fmt.Sprintf("targetpath %q is not a block device", targetPath))
}

// IsMountPoint checks if the given path is mountpoint or not.
func IsMountPoint(p string) (bool, error) {
	dummyMount := mountutils.New("")
	notMnt, err := dummyMount.IsLikelyNotMountPoint(p)
	if err != nil {
		return false, err
	}

	return !notMnt, nil
}

// filesystemNodeGetVolumeStats can be used for getting the metrics as
// requested by the NodeGetVolumeStats CSI procedure.
func filesystemNodeGetVolumeStats(
	ctx context.Context, log *logrus.Entry, targetPath string,
) (*csi.NodeGetVolumeStatsResponse, error) {
	isMnt, err := IsMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Errorf(codes.InvalidArgument, "targetpath %s does not exist", targetPath)
		}

		return nil, status.Error(codes.Internal, err.Error())
	}
	if !isMnt {
		return nil, status.Errorf(codes.InvalidArgument, "targetpath %s is not mounted", targetPath)
	}

	statfs := &unix.Statfs_t{}
	err = unix.Statfs(targetPath, statfs)
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"failed to collect FS info for mount '%s': %s", targetPath, err)
	}

	return &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Available: int64(statfs.Bavail) * int64(statfs.Bsize),
				Total:     int64(statfs.Blocks) * int64(statfs.Bsize),
				Used: (int64(statfs.Blocks) - int64(statfs.Bfree)) *
					int64(statfs.Bsize),
				Unit: csi.VolumeUsage_BYTES,
			},
			{
				Available: int64(statfs.Ffree),
				Total:     int64(statfs.Files),
				Used:      int64(statfs.Files) - int64(statfs.Ffree),
				Unit:      csi.VolumeUsage_INODES,
			},
		},
	}, nil
}

// blockNodeGetVolumeStats gets the metrics for a `volumeMode: Block` type of
// volume. At the moment, only the size of the block-device can be returned, as
// there are no secrets in the NodeGetVolumeStats request that enables us to
// connect to the Lightbits cluster.
//
// TODO: https://github.com/container-storage-interface/spec/issues/371#issuecomment-756834471
func blockNodeGetVolumeStats(ctx context.Context, log *logrus.Entry, targetPath string) (*csi.NodeGetVolumeStatsResponse, error) {
	args := []string{"--noheadings", "--bytes", "--output=SIZE", targetPath}
	lsblkSize, err := exec.CommandContext(ctx, "/bin/lsblk", args...).Output()
	if err != nil {
		err = fmt.Errorf("lsblk %v returned an error: %w", args, err)
		log.WithError(err).Error("blockNodeGetVolumeStats failed")

		return nil, status.Error(codes.Internal, err.Error())
	}

	size, err := strconv.ParseInt(strings.TrimSpace(string(lsblkSize)), 10, 64)
	if err != nil {
		err = fmt.Errorf("failed to convert %q to bytes: %w", lsblkSize, err)
		log.WithError(err).Error("blockNodeGetVolumeStats failed")

		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Total: size,
				Unit:  csi.VolumeUsage_BYTES,
			},
		},
	}, nil
}

func (d *Driver) NodeExpandVolume(
	ctx context.Context, req *csi.NodeExpandVolumeRequest,
) (*csi.NodeExpandVolumeResponse, error) {
	vid, err := ParseCSIResourceID(req.VolumeId)
	if err != nil {
		return nil, mkEinval("volume_id", err.Error())
	}

	log := d.log.WithFields(logrus.Fields{
		"op":       "NodeExpandVolume",
		"mgmt-ep":  vid.mgmtEPs,
		"vol-uuid": vid.uuid,
		"vol-path": req.VolumePath,
		"project":  vid.projName,
	})

	volumePath := req.GetVolumePath()
	if len(volumePath) == 0 {
		return nil, mkEinval("volumePath", "volume path must be provided")
	}

	reqBytes, err := getReqCapacity(req.CapacityRange)
	if err != nil {
		return nil, err
	}

	d.bdl.Lock() // TODO: break up into per-volume+per-target locks!
	defer d.bdl.Unlock()

	devicePath, err := d.getDevicePath(vid.uuid)
	if err != nil {
		return nil, err
	}

	resizer := mountutils.NewResizeFs(d.mounter.Exec)
	resizedOccurred, err := resizer.Resize(devicePath, volumePath)
	if err != nil {
		return nil, mkInternal("Could not resize volume %s (%s): %s", vid.uuid, devicePath, err)
	}
	log.Infof("resize occurred: %t. device: %q to size %v successfully",
		resizedOccurred, devicePath, reqBytes)
	return &csi.NodeExpandVolumeResponse{CapacityBytes: int64(reqBytes)}, nil
}
