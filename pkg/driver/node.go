// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// Copyright (C) 2020 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"os"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/dell/gofsutil"
	"github.com/google/uuid"
	"github.com/lightbitslabs/lb-csi/pkg/lb"
	"github.com/lightbitslabs/lb-csi/pkg/nvme-of/client/cli"
	"github.com/lightbitslabs/lb-csi/pkg/util/wait"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/kubernetes/pkg/util/resizefs"
)

// lbVolEligible() allows to rule out impossible scenarios early on. it
// checks if the volume exists on the LightOS cluster and is fully accessible
// by this host configuration-wise and in terms of target-side availability.
// all of this might change by the time we actually try to connect/mount, of
// course, but usually only for the worse, not for the better.
func (d *Driver) lbVolEligible(
	ctx context.Context, log *logrus.Entry, clnt lb.Client, vid lbVolumeID,
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

	// ACLs are normally VERY short, no point in showboating...
	for _, ace := range vol.ACL {
		if ace == d.hostNQN {
			return nil
		}
	}
	log.Warnf("volume is inaccessible from '%s', HostNQN: '%s', ACL: %#q",
		d.nodeID, d.hostNQN, vol.ACL)
	return mkPrecond("volume '%s' is inaccessible from node '%s'", vid, d.nodeID)
}

// targetEnv describes the LB environment that will be providing the underlying
// storage for a given CSI volume. in addition to the general cluster metadata,
// this includes a list of one or more relevant NVMe-oF targets. in order to
// make the volume available on a host, the host must be connected using an
// NVMe-oF protocol to at least the specified list of targets, even though at
// any given point in time only a subset of these targets might be exporting
// the volume in question. aka "target rich environment"...
type targetEnv struct {
	cluster *lb.ClusterInfo
	targets []*lb.Node // TODO: temporary hack until the Discovery Service lands
}

func (d *Driver) queryLBforTargetEnv(
	ctx context.Context, log *logrus.Entry, clnt lb.Client, vid lbVolumeID,
) (*targetEnv, error) {
	var err error
	res := &targetEnv{}
	res.cluster, err = clnt.GetClusterInfo(ctx)
	if err != nil {
		return nil, d.mungeLBErr(log, err, "failed to get info from LB cluster at '%s'",
			vid.mgmtEPs[0])
	}

	res.targets, err = clnt.ListNodes(ctx)
	if err != nil {
		return nil, d.mungeLBErr(log, err, "failed to get nodes list from LB cluster %s "+
			"at '%s'", res.cluster.UUID, vid.mgmtEPs[0])
	}

	return res, nil
}

func (d *Driver) getDevicePath(nguid uuid.UUID) (string, error) {

	client, err := cli.New(d.log) // TODO: make a member of d? otherwise cmd counting is busted!
	//                               BUT! then it's a failure on plugin startup -> no log to examine!
	if err != nil {
		return "", mkInternal("unable to gain control of NVMe-oF on the client "+
			"(host/initiator) side: %s", err)
	}

	// apparently it can take a little while between connecting (e.g.
	// 'nvme connect' successfully returning) and corresponding block
	// devices actually showing up...
	devPath := ""
	err = wait.WithRetries(30, 100*time.Millisecond, func() (bool, error) {
		devPath, err = client.GetDevPathByNGUID(nguid)
		return devPath != "", err
	})
	if err != nil || devPath == "" {
		return "", mkEExec("no local block device for volume %s", nguid)
	}
	d.log.Debugf("block device representing volume is: '%s'", devPath)
	return devPath, nil
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

	vid, err := ParseCSIVolumeID(req.VolumeId)
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
	notMnt, err := d.mounter.IsNotMountPoint(tgtPath)
	if os.IsNotExist(err) {
		return nil, mkEinvalf("staging_target_path",
			"'%s' doesn't exist", tgtPath)
	} else if err != nil {
		return nil, mkEExec("can't examine staging path: %s", err)
	}
	// don't you not like double negatives?
	if !notMnt {
		dev, _, err := mount.GetDeviceNameFromMount(d.mounter, tgtPath)
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

	////////////////////////////////////  Discovery-Client ////////////////////////////////
	entries, err := createEntries(tgtEnv.cluster.DiscoveryEndpoints, d.hostNQN, tgtEnv.cluster.SubsysNQN, "tcp")
	if err != nil {
		return nil, d.mungeLBErr(log, err, "failed to get info from LB cluster")
	}
	if err := writeEntriesToFile(vid.uuid.String(), entries); err != nil {
		return nil, d.mungeLBErr(log, err, "failed to create DC file")
	}

	devPath, err := d.getDevicePath(vid.uuid)
	if err != nil {
		return nil, err
	}

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
		return nil, mkEExec("error examining format of volume %s", vid.uuid)
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

	vid, err := ParseCSIVolumeID(req.VolumeId)
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
	vid = vid

	notMnt, err := d.mounter.IsNotMountPoint(tgtPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, mkEExec("can't examine staging path: %s", err)
	}
	if !notMnt {
		err = d.mounter.Unmount(tgtPath)
		if err != nil {
			return nil, mkEExec("failed to unmount '%s': %s", tgtPath, err)
		}
	}

	// TODO: CSI spec mandates that it is effectively CO's sacred duty to
	// do the refcounting on volumes (which is facilitated by mandating
	// idempotent semantics of the plugins). unfortunately, COs are totally
	// oblivious of the LB "all volumes grow off the same subsystem, and
	// connect/disconnect must be done per subsystem, NOT volume" semantics.
	// which means we need to devise a way of detecting when and how to
	// disconnect the subsystem, in the face of:
	// 1. CSI plugin being ephemeral and with no persistent state - that's
	//    a bit hard to track, and:
	// 2. watch out for the races: the whole connect/disconnect machinery
	//    will need to be protected by a lock, because even if we trust
	//    k8s not to run more than one instance of this plugin in the same
	//    role on the same node (do expect Node+Controller side by side,
	//    though), k8s is only too eager to invoke the gRPC calls
	//    concurrently. and, apparently, NVMe-oF does NOT track block device
	//    usage by mounts (which still sounds very odd to me!!), so in a
	//    race you can easily end up disconnecting a volume that just got
	//    mounted - with a corresponding data loss (tried it, works).
	//  so i foresee some GetDeviceNameFromMount() under lock (that func
	//  and its ilk in k8s.io-land have odd notion of TOCTTOU) around here...

	////////////////// DISCOVERY-CLIENT ///////////////////////////////////
	if err := deleteDiscoveryEntriesFile(vid.uuid.String()); err != nil {
		return nil, mkEExec("failed to delete DC file: %v", err)
	}
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (d *Driver) nodePublishVolumeForBlock(
	vid lbVolumeID, log *logrus.Entry, req *csi.NodePublishVolumeRequest,
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
	err = d.mounter.MakeFile(target)
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

func getDeviceNameFromMount(tgtPath string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info, err := gofsutil.GetMounts(ctx)
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
	vid lbVolumeID, log *logrus.Entry, req *csi.NodePublishVolumeRequest,
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
	vid, err := ParseCSIVolumeID(req.VolumeId)
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
		notMnt, err := d.mounter.IsNotMountPoint(req.TargetPath)
		if os.IsNotExist(err) {
			return nil, mkEinvalf("target_path", "'%s' doesn't exist", req.TargetPath)
		} else if err != nil {
			return nil, mkEExec("can't examine target path: %s", err)
		}
		if !notMnt {
			dev, err := getDeviceNameFromMount(req.TargetPath)
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

	vid, err := ParseCSIVolumeID(req.VolumeId)
	if err != nil {
		return nil, mkEinval("volume_id", err.Error())
	}
	vid = vid

	d.bdl.Lock() // TODO: break up into per-volume+per-target locks!
	defer d.bdl.Unlock()

	tgtPath := req.TargetPath
	notMnt, err := d.mounter.IsNotMountPoint(tgtPath)
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
					Type: csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
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
	return nil, status.Error(codes.Unimplemented, "")
}

func (d *Driver) NodeExpandVolume(
	ctx context.Context, req *csi.NodeExpandVolumeRequest,
) (*csi.NodeExpandVolumeResponse, error) {

	vid, err := ParseCSIVolumeID(req.VolumeId)
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

	devicePath, err := d.getDevicePath(vid.uuid)
	if err != nil {
		return nil, err
	}

	resizer := resizefs.NewResizeFs(d.mounter)
	resizedOccurred, err := resizer.Resize(devicePath, volumePath)
	if err != nil {
		return nil, mkInternal("Could not resize volume %s (%s): %s", vid.uuid, devicePath, err)
	}
	log.Infof("resize occurred: %t. device: %q to size %v successfully",
		resizedOccurred, devicePath, reqBytes)
	return &csi.NodeExpandVolumeResponse{CapacityBytes: int64(reqBytes)}, nil
}
