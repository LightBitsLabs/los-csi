// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"fmt"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	guuid "github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/lightbitslabs/los-csi/pkg/lb"
	"github.com/lightbitslabs/los-csi/pkg/util/endpoint"
	"github.com/lightbitslabs/los-csi/pkg/util/strlist"
)

const (
	KiB int64 = 1024
	MiB int64 = 1024 * KiB
	GiB int64 = 1024 * MiB
	TiB int64 = 1024 * GiB
	PiB int64 = 1024 * TiB
)

const (
	volCapGranularity int64 = GiB
	minVolCap         int64 = volCapGranularity
)

var (
	controllerCaps = []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
	}
	capsCache []*csi.ControllerServiceCapability
)

func init() {
	// ah, the wonders of flat structuring and concise naming...
	for _, cap := range controllerCaps {
		capsCache = append(capsCache,
			&csi.ControllerServiceCapability{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: cap,
					},
				},
			},
		)
	}
}

func (d *Driver) ControllerGetCapabilities(
	_ context.Context, _ *csi.ControllerGetCapabilitiesRequest,
) (*csi.ControllerGetCapabilitiesResponse, error) {
	// a bit of a conundrum... in order to implement the GetCapacity()
	// CSI API entrypoint, this plugin needs the ability to authenticate
	// to a LightOS server, therefore it needs a JWT. however, the
	// GetCapacity() CSI entrypoint doesn't include a `secrets` param. so,
	// this can only work if the plugin is running in a global JWT mode
	// (i.e. a single JWT file is specified at deployment time via the
	// $LB_CSI_JWT_PATH env var or the `--jwt-path` cmd-line arg, though
	// the contents of the file are monitored for changes at runtime).
	//
	// having ControllerGetCapabilities() response potentially vary from
	// call to call is not ideal, especially since some/most COs might not
	// even bother calling it more than once. unfortunately, this seems
	// like the best balance between allowing at least some deployments
	// (those running in global JWT mode) to "enjoy" GetCapacity() access,
	// and not misleadingly reporting bogus support for it on other
	// deployments, where it'll just fail on authZ errors...
	//
	// the whole secrets/credentials story both in CSI and K8s could
	// certainly use some fixing...
	caps := capsCache
	if d.jwt != "" {
		caps = append([]*csi.ControllerServiceCapability{
			&csi.ControllerServiceCapability{ //nolint:gofumpt // tool version diffs
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_GET_CAPACITY,
					},
				},
			},
		}, caps...)
	}
	return &csi.ControllerGetCapabilitiesResponse{Capabilities: caps}, nil
}

func getReqCapacity(capRange *csi.CapacityRange) (uint64, error) {
	// potentially those two might be cluster-specific in the future, so we'd
	// need to grab them using the LightOS mgmt API first:
	volCap := minVolCap
	capGran := volCapGranularity

	if capRange == nil {
		return uint64(volCap), nil
	}

	minCap := capRange.RequiredBytes
	maxCap := capRange.LimitBytes
	if minCap < 0 || maxCap < 0 {
		return 0, mkEinvalf(capRangeField,
			"invalid range specified: [%d..%d]", minCap, maxCap)
	}
	if minCap == 0 && maxCap == 0 {
		return 0, mkEinvalf(capRangeField,
			"both 'required_bytes' and 'limit_bytes' are missing")
	}
	if maxCap != 0 && maxCap < minVolCap {
		return 0, mkErange("bad value of '%s': %dB is below minimum volume size of %dB",
			capRangeLimField, maxCap, minVolCap)
	}
	if minCap != 0 && maxCap != 0 && maxCap < minCap {
		return 0, mkEinvalf(capRangeField,
			"invalid range specified: [%d..%d]", minCap, maxCap)
	}
	volCap = (minCap + capGran - 1) / capGran * capGran
	if maxCap != 0 && volCap > maxCap {
		return 0, mkErange("bad value of '%s': capacity granularity is %d bytes, "+
			"can't create volume of capacity range: [%d..%d]",
			capRangeField, capGran, minCap, maxCap)
	}

	return uint64(volCap), nil
}

func mkVolumeResponse(
	mgmtEPs endpoint.Slice, vol *lb.Volume, mgmtScheme string, volSrc *csi.VolumeContentSource,
) *csi.CreateVolumeResponse {
	volID := lbResourceID{
		mgmtEPs:  mgmtEPs,
		uuid:     vol.UUID,
		projName: vol.ProjectName,
		scheme:   mgmtScheme,
	}
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			CapacityBytes: int64(vol.Capacity),
			VolumeId:      volID.String(),
			ContentSource: volSrc,
		},
	}
}

func chkContentSourceCompat(
	srcReplicaCount uint32, srcCompression bool, srcCapacity uint64,
	req lb.Volume, reqCapacity csi.CapacityRange, field string,
) error {
	if req.ReplicaCount != srcReplicaCount {
		return mkEinvalf(field, "requested volume replica count of %d differs from content "+
			"source count of %d", req.ReplicaCount, srcReplicaCount)
	}
	if req.Compression != srcCompression {
		b2s := map[bool]string{false: "disabled", true: "enabled"}
		return mkEinvalf(field, "requested volume with %s compression from content source "+
			"with %s compression", b2s[req.Compression], b2s[srcCompression])
	}

	// LightOS supports creating volumes that are bigger or equal in size than
	// the base snapshot. however, exposing the ability to create volume clones
	// bigger than the original snapshot/volume through the CSI plugin is a
	// little tricky: FS/mount volumes would first have to have their FS
	// resized. that would need to be done specifically when they're attached
	// to a node for the first time. tracking all this stuff reliably is a bit
	// of a pain in a stateless system.
	//
	// TODO: consider adding support for the above, special-casing FS vs block.
	minCap := uint64(reqCapacity.RequiredBytes)
	maxCap := uint64(reqCapacity.LimitBytes)
	if minCap > srcCapacity || maxCap != 0 && maxCap < srcCapacity {
		rLim := "2^64)" // excl.
		if maxCap != 0 {
			rLim = fmt.Sprintf("%d]", maxCap) // incl.
		}
		return mkErange("volume content source size of %dB is outside the requested "+
			"capacity range: [%d..%s", srcCapacity, minCap, rLim)
	}
	return nil
}

// chkSourceSnapCompat() checks whether the source snapshot specified by UUID
// in the `req` volume description:
// * exists on the LightOS cluster,
// * is in a usable state for creating a volume from it,
// * has parameters compatible with those of the volume to be created from it,
//   except for the capacity, which is taken directly from the CSI request
//   `reqCapacity` param piped through to here.
// if so - it UPDATES the `req` volume capacity in-situ to match that of the
// source snapshot and returns nil, otherwise returns a gRPC Status error
// suitable for direct return to the callers of CreateVolume().
func chkSourceSnapCompat(
	ctx context.Context, log *logrus.Entry, clnt lb.Client, req *lb.Volume,
	reqCapacity csi.CapacityRange, srcSid lbResourceID,
) error {
	snap, err := clnt.GetSnapshot(ctx, srcSid.uuid, srcSid.projName)
	if err != nil {
		if isStatusNotFound(err) {
			return mkEnoent("content source snapshot %s doesn't exist", srcSid.uuid)
		}
		return mungeLBErr(log, err, "failed to get content source snapshot %s from LB",
			srcSid.uuid)
	}

	switch snap.State {
	case lb.SnapshotAvailable:
		// the only state in which a snapshot can serve as a content source.
	case lb.SnapshotDeleting:
		return mkEinvalf(volContSrcSnapField, "content source snapshot %s "+
			"is in the process of being deleted", snap.UUID)
	case lb.SnapshotCreating:
		return mkEagain("content source snapshot %s is still being created", snap.UUID)
	default:
		return mkInternal("found content source snapshot %s in unexpected state '%s' (%d)",
			snap.UUID, snap.State, snap.State)
	}

	err = chkContentSourceCompat(snap.SrcVolReplicaCount, snap.SrcVolCompression,
		snap.Capacity, *req, reqCapacity, volContSrcSnapField)
	if err != nil {
		return err
	}
	req.Capacity = snap.Capacity
	return nil
}

// chkSourceVolCompat() checks whether the source volume specified by `srcVid`
// exists is usable and compatible with the volume to be created from it,
// specified by `req` - except for the explicit `reqCapacity`. for more details
// see chkSourceSnapCompat().
//
// if the two are compatible, it UPDATES the `req` volume capacity in-situ to
// match that of the source volume and returns nil, otherwise returns a gRPC
// Status error suitable for direct return to the callers of CreateVolume().
func chkSourceVolCompat(
	ctx context.Context, log *logrus.Entry, clnt lb.Client, req *lb.Volume,
	reqCapacity csi.CapacityRange, srcVid lbResourceID,
) error {
	vol, err := clnt.GetVolume(ctx, srcVid.uuid, srcVid.projName)
	if err != nil {
		if isStatusNotFound(err) {
			return mkEnoent("source volume %s doesn't exist", srcVid.uuid)
		}
		return mungeLBErr(log, err, "failed to get info of source volume %s from LB: %s",
			srcVid.uuid)
	}

	switch vol.State {
	case lb.VolumeAvailable:
		// the only state in which a volume can serve as a content source.
	case lb.VolumeDeleting:
		return mkEinvalf(volContSrcVolField, "content source volume %s "+
			"is in the process of being deleted", vol.UUID)
	case lb.VolumeUpdating:
		return mkEagain("content source volume %s is being updated", vol.UUID)
	default:
		return mkInternal("found snapshot '%s' (%s) in unexpected state '%s' (%d)",
			vol.Name, vol.UUID, vol.State, vol.State)
	}

	err = chkContentSourceCompat(vol.ReplicaCount, vol.Compression,
		vol.Capacity, *req, reqCapacity, volContSrcVolField)
	if err != nil {
		return err
	}
	req.Capacity = vol.Capacity
	return nil
}

// findExistingVolume() tries to look up [by name] in a LightOS cluster an existing
// volume that would be "CSI compatible" with `req` details (this would typically
// be a result of a retry from a CO). if `srcVid` or (as in "xor") `srcSid` is
// specified, checks that the volume is actually sourced from the specified volume
// or snapshot, and that is capacity is within `reqCapacity`, rather than matching
// req.Capacity precisely.
//
// if a matching volume is found - returns its volume descriptor, nil otherwise,
// which is NOT considered an error. all errors returned are gRPC-compatible and
// should probably be returned verbatim by the caller CreateVolume().
func findExistingVolume(
	ctx context.Context, log *logrus.Entry, clnt lb.Client, req lb.Volume,
	reqCapacity csi.CapacityRange, srcVid, srcSid *lbResourceID,
) (*lb.Volume, error) {
	vol, err := clnt.GetVolumeByName(ctx, req.Name, req.ProjectName)
	if err != nil {
		if isStatusNotFound(err) {
			return nil, nil
		}
		return nil, mungeLBErr(log, err,
			"failed to check if volume '%s' already exists on LB", req.Name)
	}
	log = log.WithField("vol-uuid", vol.UUID)

	switch vol.State {
	case lb.VolumeAvailable, lb.VolumeUpdating, lb.VolumeCreating:
		// might be usable, now or later - if it otherwise matches.
		// see a second check down below.
	case lb.VolumeDeleting:
		// volume deletion in LB is a background operation that might take
		// some time. this shouldn't prevent a new volume from being created,
		// but it's handy to know this happened. just in case... ;)
		log.Infof("spotted appropriately named volume in the process of being deleted, " +
			"ignoring it")
		return nil, nil
	default:
		return nil, mkInternal("volume '%s' exists but is in unexpected "+
			"state '%s' (%d)", vol.Name, vol.State, vol.State)
	}

	// provenance check:
	prefix := fmt.Sprintf("volume '%s' exists but is incompatible", vol.Name)
	if srcSid != nil {
		if vol.SnapshotUUID != srcSid.uuid {
			if vol.SnapshotUUID == guuid.Nil {
				return nil, mkEExist("%s: it is not based on snapshot %s as requested",
					prefix, srcSid.uuid)
			}
			return nil, mkEExist("%s: it is based on snapshot %s instead of the "+
				"requested one: %s", prefix, vol.SnapshotUUID, srcSid.uuid)
		}
	} else if srcVid != nil {
		if vol.SnapshotUUID == guuid.Nil {
			return nil, mkEExist("%s: it was required to be based on volume %s, but has "+
				"no requisite intermedite snapshot listed as base", prefix, srcVid.uuid)
		}
		snap, err := clnt.GetSnapshot(ctx, vol.SnapshotUUID, vol.ProjectName)
		if err != nil {
			return nil, mungeLBErr(log, err, "failed to get snapshot %s from LB while "+
				"checking existing volume '%s' provenance",
				vol.SnapshotUUID, vol.Name)
		}
		if snap.SrcVolUUID != srcVid.uuid {
			return nil, mkEExist("%s: it is based on volume %s instead of the "+
				"requested one: %s", prefix, snap.SrcVolUUID, srcVid.uuid)
		}
	}

	diffs := req.ExplainDiffsFrom(vol, "requested", "actual",
		lb.SkipUUID|lb.SkipSnapUUID|lb.SkipCapacity)
	if len(diffs) > 0 {
		return nil, mkEExist("%s: %s", prefix, strings.Join(diffs, ", "))
	}
	if !strlist.AreEqual(vol.ACL, req.ACL) {
		// this is likely a race with some other instance, but
		// the ACL should be properly adjusted afterwards, on
		// ControllerPublishVolume()/ControllerUnpublishVolume()
		// (which the other instance may have already reached).
		log.Warnf("found matching existing volume with "+
			"unexpected ACL %#q instead of %#q", vol.ACL, req.ACL)
	}

	if srcSid != nil || srcVid != nil {
		if vol.Capacity < uint64(reqCapacity.RequiredBytes) {
			return nil, mkEExist("%s: its capacity of %dB is smaller than the required %dB",
				prefix, vol.Capacity, reqCapacity.RequiredBytes)
		}
		if reqCapacity.LimitBytes != 0 && vol.Capacity > uint64(reqCapacity.LimitBytes) {
			return nil, mkEExist("%s: its capacity of %dB is bigger than the %dB limit",
				prefix, vol.Capacity, reqCapacity.LimitBytes)
		}
	} else if vol.Capacity != req.Capacity {
		return nil, mkEExist("%s: its capacity of %dB differs from the requred %dB rounded up"+
			prefix, vol.Capacity, req.Capacity)
	}

	if vol.State == lb.VolumeCreating {
		return nil, mkEagain("volume '%s' is still being created", vol.Name)
	}

	// TODO: check protection state and stall with EAGAIN, making
	// the CO retry, if it's read-only on unavailable?
	// this way the user workload will be DELAYED, possibly for a
	// long time - until the relevant LightOS cluster nodes are up
	// and caught up - but at least will not see EIO right off the
	// bat (that might still happen on massive LightOS cluster node
	// outages after the volume is created and returned, of course)...

	// if reached here - a matching volume already exists...
	if !vol.IsWritable() {
		log.Warnf("volume already exists, but is not currently usable: "+
			"its protection state is '%s'", vol.Protection)
	} else {
		log.Info("volume already exists")
	}
	return vol, nil
}

// doCreateVolume() actually creates a new volume based on the CO requirements,
// possibly basing it on an existing snapshot or volume (indirectly).
//
// if `srcSid` is specified, doCreateVolume() will base the volume on that
// snapshot, if it's compatible with the requested parameters of the new volume,
// including having capacity that satisfies `reqCapacity. in this case the new
// volume capacity will derived from the source snapshot capacity, and might
// differ from `req.Capacity`.
//
// alternatively, if `srcVid` is specified doCreateVolume() will try to take a
// temporary snapshot of the source volume, and base the new volume on that.
// that's because currently LB volumes can only be based on explicit snapshots.
// the temporary snapshot will be immediately auto-deleted, though it will
// currently linger in the 'Deleted' state until the volume based on it disappears.
// similar to the `srcSid` case, the resultant volume capacity will be based on
// that of the source.
func (d *Driver) doCreateVolume(
	ctx context.Context, log *logrus.Entry, clnt lb.Client, req lb.Volume,
	reqCapacity csi.CapacityRange, srcVid, srcSid *lbResourceID,
) (*lb.Volume, error) {
	// see if it's a "clone" request (creating a volume from another volume or
	// a snapshot), and if so - figure out the UUID of a snapshot to base the
	// new volume on, if necessary - creating a temporary snapshot in the process.
	//
	// TODO: BEFORE doing anything else (like trying to create intermediate
	// snapshot or a new volume), the flow should probably call GetClusterInfo()
	// first using `clnt` (that was created for mgmtEPs derived from the original
	// `req.Parameters`), then, unless the mgmtEPs are IDENTICAL, create a
	// separate LB client for mgmtEPs derived from the `VolumeContentSource`
	// lbResourceID and call GetClusterInfo() on that, to verify that both the
	// content source and the volume being requested belong to the same LB
	// cluster (have identical SubsysNQNs), as cross-cluster volume creation is
	// nonsensical. unfortunately, string comparisons on mgmtEPs won't be of much
	// help due to cluster resizing, VIPs, DNS, plugin evolution in the face of
	// long-lived volumes, etc.
	if srcVid != nil {
		err := chkSourceVolCompat(ctx, log, clnt, &req, reqCapacity, *srcVid)
		if err != nil {
			return nil, err
		}

		// TODO: this "intermediate snapshot" naming logic is problematic: it
		// ties the origin snapshot content to the name of the target volume,
		// instead of to the point in time at which the snapshot was requested,
		// in some cases this will result in the new volume being created from
		// contents possibly hours, days or months out of date. due to the lazy
		// snapshot deletion strategy this will also prevent creation of
		// identically named clone volumes in the future due to snapshot name
		// collisions (see note below on cleanup).
		snapName := "snapshot-" + req.Name
		log.Infof("auto-creating intermediate snapshot '%s' to clone from a volume", snapName)
		snap, err := doCreateSnapshot(ctx, log, clnt, snapName, *srcVid,
			"auto-snap for clone, by: LB CSI")
		if err != nil {
			return nil, prefixErr(err,
				"failed to create intermediate snapshot to be used as content source")
		}

		tmpSid := lbResourceID{
			mgmtEPs:  srcVid.mgmtEPs,
			uuid:     snap.UUID,
			projName: snap.ProjectName,
			scheme:   srcVid.scheme,
		}

		// TODO: LB doesn't support deletion of snapshots with live volumes
		// based on them. LB will accept this request and the snapshot will
		// enter 'Deleting' state, but the snapshot will remain until the
		// last "derived" volume disappears. moreover, snapshot deletion might
		// take some time from the point in time where the snapshot becomes
		// eligible for final removal from the system.
		//
		// as a result, some flows might experience unexpected results. e.g.
		// "cloning" a volume through such an intermediate snapshot, using
		// the clone for a while, deleting the clone, then immediately
		// trying to create an identically named clone of the original
		// volume again (but, presumably, based on the up-to-date contents
		// of the original volume) will likely fail: using the current
		// intermediate snapshot naming scheme, the second clone operation
		// will just find the OLD snapshot in the 'Deleting' state...
		defer func() {
			log.Infof("requesting auto-deletion of intermediate snapshot '%s'", snapName)

			// yes, this is definitely not atomic with respect to creation above...
			err = doDeleteSnapshot(ctx, log, clnt, tmpSid)
			if err != nil {
				// TODO: theoretically, if LightOS returned one of the
				// "temporary" errors, this method could spawn a
				// Goroutine that would keep retrying to delete the
				// intermediate snapshot in the background. however,
				// there are no guarantees that this process will
				// remain alive long enough, that a multitude of such
				// background tasks won't pile up on K8s retries, etc.

				// doesn't justify failing CreateVolume(), caller got vol.
				log.Errorf("auto-deletion of intermediate snapshot '%s' failed: %s",
					snapName, err)
			}
		}()

		req.SnapshotUUID = snap.UUID
	} else if srcSid != nil {
		err := chkSourceSnapCompat(ctx, log, clnt, &req, reqCapacity, *srcSid)
		if err != nil {
			return nil, err
		}
		req.SnapshotUUID = srcSid.uuid
	}

	vol, err := clnt.CreateVolume(ctx, req.Name, req.Capacity, req.ReplicaCount,
		req.Compression, req.ACL, req.ProjectName, req.SnapshotUUID, true)
	if err != nil {
		return nil, mungeLBErr(log, err, "failed to create volume '%s'", req.Name)
	}

	log.WithField("vol-uuid", vol.UUID).Info("volume created successfully")
	return vol, nil
}

// CreateVolume uses info extracted from request `parameters` field to connect
// to LB and attempt to create the volume specified by `name` field (or return
// info on an existing volume if one matches, for idempotency). see
// `lbCreateVolumeParams` for more details on the format.
//
// TODO: on volume "clone" ops, the CSI spec seems to require that the plugin
// detect "incompatibility between `parameters` from the source and the ones
// requested for the new volume". unfortunately, the CSI spec does NOT supply
// the `parameters` of the source as part of the params to this call. it is
// unspecified how the plugin is supposed to pull this off.
//
// TODO: it's also unclear what's the expected behaviour if a "clone" op is
// requested for a FS volume with a block mode vol or snap specified as a
// source. since the capabilities of the source volume are not passed in to
// this call either, how are plugins expected to detect such cases?
func (d *Driver) CreateVolume(
	ctx context.Context, req *csi.CreateVolumeRequest,
) (*csi.CreateVolumeResponse, error) {
	if req.Name == "" {
		return nil, mkEinvalMissing("name")
	}
	if req.AccessibilityRequirements != nil {
		return nil, mkEinval("accessibility_requirements",
			"accessibility constraints are not supported")
	}
	// `capacity` will be used for creating new free-standing volumes:
	capacity, err := getReqCapacity(req.CapacityRange)
	if err != nil {
		return nil, err
	}
	// reqCapacity is the acceptable capacity range, consulted in "clone" cases:
	reqCapacity := csi.CapacityRange{}
	if req.CapacityRange != nil {
		reqCapacity = *req.CapacityRange
	}
	if err = d.validateVolumeCapabilities(req.VolumeCapabilities); err != nil {
		return nil, err
	}
	params, err := parseCSICreateVolumeParams(req.Parameters)
	if err != nil {
		return nil, err
	}

	log := d.log.WithFields(logrus.Fields{
		"op":       "CreateVolume",
		"mgmt-ep":  params.mgmtEPs,
		"vol-name": req.Name,
		"project":  params.projectName,
	})

	volSrc := req.VolumeContentSource
	var srcVid, srcSid *lbResourceID
	if vol := volSrc.GetVolume(); vol != nil {
		vid, err := parseCSIResourceIDEnoent(volContSrcVolField, vol.VolumeId)
		if err != nil {
			return nil, err
		}
		if vid.projName != params.projectName {
			return nil, mkEinvalf(volContSrcVolField, "can't create volume in project "+
				"'%s' from volume in project '%s'", params.projectName, vid.projName)
		}
		srcVid = &vid
		log = log.WithField("src-vol-uuid", vid.uuid)
	} else if snap := volSrc.GetSnapshot(); snap != nil {
		sid, err := parseCSIResourceIDEnoent(volContSrcSnapField, snap.SnapshotId)
		if err != nil {
			return nil, err
		}
		if sid.projName != params.projectName {
			return nil, mkEinvalf(volContSrcSnapField, "can't create volume in project "+
				"'%s' from snapshot in project '%s'", params.projectName, sid.projName)
		}
		srcSid = &sid
		log = log.WithField("src-snap-uuid", sid.uuid)
	} else if volSrc != nil {
		if volSrc.Type != nil {
			return nil, mkEinvalf(volContSrcField, "unsupported content source type '%T'",
				volSrc.Type)
		}
		volSrc = nil
	}
	// from here on: if volSrc != nil - it's definitely a clone request.

	wantVol := lb.Volume{
		Name:         req.Name,
		Capacity:     capacity, // NOTE: might be updated to that of the content source!
		ReplicaCount: params.replicaCount,
		Compression:  params.compression,
		ACL:          []string{lb.ACLAllowNone},
		ProjectName:  params.projectName,
	}

	ctx = d.cloneCtxWithCreds(ctx, req.Secrets)
	clnt, err := d.GetLBClient(ctx, params.mgmtEPs, params.mgmtScheme)
	if err != nil {
		return nil, err
	}
	defer d.PutLBClient(clnt)

	// check if a matching volume already exists (likely a result of retry from CO):
	vol, err := findExistingVolume(ctx, log, clnt, wantVol, reqCapacity, srcVid, srcSid)
	if err != nil {
		return nil, err
	}
	if vol == nil {
		// ...nope, need to actually create a new volume:
		vol, err = d.doCreateVolume(ctx, log, clnt, wantVol, reqCapacity, srcVid, srcSid)
		if err != nil {
			return nil, err
		}
	}
	return mkVolumeResponse(params.mgmtEPs, vol, params.mgmtScheme, volSrc), nil
}

func (d *Driver) ControllerGetVolume(
	_ context.Context, _ *csi.ControllerGetVolumeRequest,
) (*csi.ControllerGetVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (d *Driver) DeleteVolume(
	ctx context.Context, req *csi.DeleteVolumeRequest,
) (*csi.DeleteVolumeResponse, error) {
	log := d.log.WithField("op", "DeleteVolume")
	vid, err := parseCSIResourceIDEnoent(volIDField, req.VolumeId)
	if err != nil {
		if isStatusNotFound(err) {
			log.Errorf("bad value of '%s': %s", volIDField, err)
			// returning success instead of error to pacify some of the more
			// simple-minded external tests:
			return &csi.DeleteVolumeResponse{}, nil
		}
		return nil, err
	}

	log = d.log.WithFields(logrus.Fields{
		"mgmt-ep":  vid.mgmtEPs,
		"vol-uuid": vid.uuid,
		"project":  vid.projName,
	})

	ctx = d.cloneCtxWithCreds(ctx, req.Secrets)
	clnt, err := d.GetLBClient(ctx, vid.mgmtEPs, vid.scheme)
	if err != nil {
		return nil, err
	}
	defer d.PutLBClient(clnt)

	// TODO: ideally, the GetVolume()/switch-State thing below should go
	// away, the code should just blindly call LB mgmt API DeleteVolume()
	// (in a "blocking" mode of the lbgrpc client that will wait for the
	// volume to transition to "Deleting" state before returning), and if
	// LB returns codes.NotFound - this func should return success and be
	// done with it.
	// unfortunately, currently, LB mgmt API returns "codes.Internal" on
	// all DeleteVolume() "errors", including being called on non-existent
	// volume, so there's no way to distinguish non-existent volume from
	// cluster-side problems from malformed params, etc. hence the below...

	// maybe it's already gone:
	vol, err := clnt.GetVolume(ctx, vid.uuid, vid.projName)
	if err != nil {
		if isStatusNotFound(err) {
			log.Info("volume already gone")
			return &csi.DeleteVolumeResponse{}, nil
		}
		// TODO: for the time being this case should probably cause a
		// retry, not so sure about that long term...
		return nil, mkEagain("failed to get volume %s from LB", vid.uuid)
	}

	// or maybe it's in the process of being deleted, or can't be deleted:
	switch vol.State {
	case lb.VolumeAvailable:
		// this is really the only one that can and should be deleted.
	case lb.VolumeDeleting,
		// VolumeFailed is currently a bit of a quirk: it means that
		// a volume creation attempt failed, the volume's husk is still
		// visible in the system, but it can't be deleted, and a new
		// volume with the same name can be created... <shrug>
		lb.VolumeFailed:
		log.Info("volume effectively already gone")
		return &csi.DeleteVolumeResponse{}, nil
	case lb.VolumeCreating:
		return nil, mkEagain("volume %s is still being created", vol.UUID)
	case lb.VolumeUpdating:
		return nil, mkEagain("volume %s is being updated", vol.UUID)
	default:
		return nil, mkInternal("found volume '%s' (%s) in unexpected state '%s' (%d)",
			vol.Name, vol.UUID, vol.State, vol.State)
	}

	// TODO: consider checking if the volume is in use at this time? e.g.
	// using connectedHosts field of the volume? it's not exactly exact
	// science, what with TOCTTOU, and all, but still might avoid accidental
	// user data loss...

	// oh, well, just delete it:
	err = clnt.DeleteVolume(ctx, vol.UUID, vid.projName, true)
	if err != nil {
		// TODO: examine and convert the error if necessary:
		return nil, err
	}

	log.Info("volume deleted")
	return &csi.DeleteVolumeResponse{}, nil
}

func (d *Driver) ControllerPublishVolume(
	ctx context.Context, req *csi.ControllerPublishVolumeRequest,
) (*csi.ControllerPublishVolumeResponse, error) {
	// NOTE: preserve the order of params checks (see below)!
	if req.VolumeId == "" {
		return nil, mkEinvalMissing(volIDField)
	}
	// vidErr is checked down below, because we want the extra fields in the
	// log whenever we can get them, while csi-sanity makes IMPLICIT
	// REQUIREMENTS on the CSI plugins about the order in which the plugins
	// must be checking their params when it supplies multiple bogus
	// arguments at once.
	vid, vidErr := parseCSIResourceID(req.VolumeId)

	log := d.log.WithFields(logrus.Fields{
		"op":       "ControllerPublishVolume",
		"mgmt-ep":  vid.mgmtEPs,
		"vol-uuid": vid.uuid,
		"project":  vid.projName,
	})

	if err := checkNodeID(req.NodeId); err != nil {
		// NodeId should have been previously validated by the node
		// instance of the driver upon start-up and returned from
		// NodeGetInfo(), so this is odd...
		d.log.Errorf("unexpected node_id: '%s'", req.NodeId)
		return nil, mkEinval(nodeIDField, err.Error())
	}
	// TODO: this one is tricky, because the CSI plugin is supposed to be
	// able to figure out if the volume has previously been published to
	// the node in question with an incompatible `volume_capability` (e.g.
	// as block volume instead of FS mount, presumably?). except how would
	// THIS instance know about what went on a long time ago on a node
	// far, far away?
	if err := d.validateVolumeCapability(req.VolumeCapability); err != nil {
		return nil, err
	}
	if vidErr != nil {
		return nil, mkEnoent("bad value of '%s': %s", volIDField, vidErr)
	}
	if req.Readonly {
		return nil, mkEinval("readonly", "read-only volumes are not supported")
	}

	log = log.WithField("node-id", req.NodeId)
	ace := nodeIDToHostNQN(req.NodeId)

	ctx = d.cloneCtxWithCreds(ctx, req.Secrets)
	clnt, err := d.GetLBClient(ctx, vid.mgmtEPs, vid.scheme)
	if err != nil {
		return nil, err
	}
	defer d.PutLBClient(clnt)

	publishVolumeHook := func(vol *lb.Volume) (*lb.VolumeUpdate, error) {
		log = log.WithField("acl-curr", fmt.Sprintf("%#q", vol.ACL))
		// currently we support only SINGLE_NODE_WRITER/RWO, so it's
		// pretty simple:
		numACEs := len(vol.ACL)
		if numACEs == 1 && vol.ACL[0] == ace {
			log.Info("volume is already published to node")
			return nil, nil
		}
		if numACEs == 0 || numACEs == 1 && vol.ACL[0] == lb.ACLAllowNone {
			return &lb.VolumeUpdate{ACL: []string{ace}}, nil
		}

		log.Error("volume is already published to other node(s)")
		if numACEs == 1 && vol.ACL[0] == lb.ACLAllowAny {
			// we'd have never done this, so, that leaves an admin
			// intervention or an improperly statically provisioned
			// volume?
			return nil, mkPrecond("volume has a mis-configured ACL: %#q", vol.ACL)
		}
		nodes := make([]string, len(vol.ACL))
		for i, a := range vol.ACL {
			if node := hostNQNToNodeID(a); node != "" {
				nodes[i] = node
			} else {
				nodes[i] = a
			}
		}
		return nil, mkPrecond("volume is already published to nodes: '%s'",
			strings.Join(nodes, "', '"))
	}

	vol, err := clnt.UpdateVolume(ctx, vid.uuid, vid.projName, publishVolumeHook)
	if err != nil {
		return nil, err
	}
	if len(vol.ACL) != 1 || vol.ACL[0] != ace {
		// either some race involving network partitions, or, an
		// external intervention. a retry will either sort it out, or
		// report the condition more accurately:
		log.WithFields(logrus.Fields{
			"acl-exp": fmt.Sprintf("%#q", []string{ace}),
			"acl-got": fmt.Sprintf("%#q", vol.ACL),
		}).Errorf("UpdateVolume() succeeded, but resultant volume ACL is wrong")
		return nil, mkEagain("failed to publish volume to node '%s'", req.NodeId)
	}
	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (d *Driver) ControllerUnpublishVolume(
	ctx context.Context, req *csi.ControllerUnpublishVolumeRequest,
) (*csi.ControllerUnpublishVolumeResponse, error) {
	vid, err := parseCSIResourceIDEinval(volIDField, req.VolumeId)
	if err != nil {
		return nil, err
	}

	log := d.log.WithFields(logrus.Fields{
		"op":       "ControllerUnpublishVolume",
		"mgmt-ep":  vid.mgmtEPs,
		"vol-uuid": vid.uuid,
		"project":  vid.projName,
	})

	if err := checkNodeID(req.NodeId); err != nil {
		// NodeId should have been previously validated by the node
		// instance of the driver upon start-up and returned from
		// NodeGetInfo(), so this is odd...
		d.log.Errorf("unexpected node_id: '%s'", req.NodeId)
		return nil, mkEinval("node_id", err.Error())
	}

	log = log.WithField("node-id", req.NodeId)
	ace := nodeIDToHostNQN(req.NodeId)
	allowNoneACL := []string{lb.ACLAllowNone}

	ctx = d.cloneCtxWithCreds(ctx, req.Secrets)
	clnt, err := d.GetLBClient(ctx, vid.mgmtEPs, vid.scheme)
	if err != nil {
		return nil, err
	}
	defer d.PutLBClient(clnt)

	unpublishVolumeHook := func(vol *lb.Volume) (*lb.VolumeUpdate, error) {
		log = log.WithField("acl-curr", fmt.Sprintf("%#q", vol.ACL))
		// currently we support only SINGLE_NODE_WRITER/RWO, so it's
		// pretty simple:
		numACEs := len(vol.ACL)
		if numACEs == 1 && vol.ACL[0] == lb.ACLAllowNone {
			log.Info("volume is already not published to any node")
			return nil, nil
		}
		if numACEs == 0 || numACEs == 1 && vol.ACL[0] == ace {
			return &lb.VolumeUpdate{ACL: allowNoneACL}, nil
		}

		if numACEs > 1 || numACEs == 1 && vol.ACL[0] == lb.ACLAllowAny {
			// we'd have never done this, so, that leaves an admin
			// intervention, an improperly statically provisioned
			// volume, or, in case of multiple ACLS, possibly a bug.
			return nil, mkInternal("volume has a mis-configured ACL: %#q", vol.ACL)
		}

		log.Warn("volume is already published to another node")
		return nil, nil
	}

	vol, err := clnt.UpdateVolume(ctx, vid.uuid, vid.projName, unpublishVolumeHook)
	if err != nil {
		if isStatusNotFound(err) {
			log.Info("volume is already gone, unpublishing is irrelevant")
			return &csi.ControllerUnpublishVolumeResponse{}, nil
		}
		return nil, err
	}
	numACEs := len(vol.ACL)
	if numACEs > 1 || numACEs == 1 && vol.ACL[0] == ace {
		// either some race involving network partitions, or, an
		// external intervention. a retry will either sort it out, or
		// report the condition more accurately:
		log.WithFields(logrus.Fields{
			"acl-exp": fmt.Sprintf("%#q", allowNoneACL),
			"acl-got": fmt.Sprintf("%#q", vol.ACL),
		}).Errorf("UpdateVolume() succeeded, but resultant volume ACL is wrong")
		return nil, mkEagain("failed to unpublish volume from node '%s'", req.NodeId)
	}
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (d *Driver) ValidateVolumeCapabilities(
	ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest,
) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	vid, err := parseCSIResourceIDEnoent(volIDField, req.VolumeId)
	if err != nil {
		return nil, err
	}
	if req.VolumeCapabilities == nil {
		return nil, mkEinvalMissing("volume_capabilities")
	}

	log := d.log.WithFields(logrus.Fields{
		"op":       "ValidateVolumeCapabilities",
		"mgmt-ep":  vid.mgmtEPs,
		"vol-uuid": vid.uuid,
		"project":  vid.projName,
	})

	ctx = d.cloneCtxWithCreds(ctx, req.Secrets)
	clnt, err := d.GetLBClient(ctx, vid.mgmtEPs, vid.scheme)
	if err != nil {
		return nil, err
	}
	defer d.PutLBClient(clnt)

	if _, err := clnt.GetVolume(ctx, vid.uuid, vid.projName); err != nil {
		if isStatusNotFound(err) {
			return nil, mkEnoent("volume '%s' doesn't exist", vid)
		}
		return nil, mungeLBErr(log, err, "failed to get volume '%s' from LB", vid)
	}

	if err = d.validateVolumeCapabilities(req.VolumeCapabilities); err != nil {
		return nil, err
	}
	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeContext:      req.VolumeContext,
			VolumeCapabilities: req.VolumeCapabilities,
			Parameters:         req.Parameters,
		},
	}, nil
}

func (d *Driver) ListVolumes(
	_ context.Context, _ *csi.ListVolumesRequest,
) (*csi.ListVolumesResponse, error) {
	// TODO: er... impl?

	return nil, status.Error(codes.Unimplemented, "")
}

func (d *Driver) GetCapacity(
	ctx context.Context, req *csi.GetCapacityRequest,
) (*csi.GetCapacityResponse, error) {
	if caps := req.GetVolumeCapabilities(); caps != nil {
		if err := d.validateVolumeCapabilities(caps); err != nil {
			return nil, err
		}
	}

	// non-cluster-specific requests are meaningless:
	if req.Parameters == nil {
		return &csi.GetCapacityResponse{AvailableCapacity: 0}, nil
	}
	// a minor hack to allow querying capacity without having to specify
	// the replication factor (other non-vital params have defaults):
	if rc := req.Parameters[volParRepCntKey]; rc == "" {
		req.Parameters[volParRepCntKey] = "1"
	}
	params, err := parseCSICreateVolumeParams(req.Parameters)
	if err != nil {
		return nil, err
	}

	// a major hack: the CSI API GetCapacity() entrypoint doesn't have a
	// `secrets` param, so unless the CSI plugin is running in a global
	// JWT mode (i.e. a single JWT is specified via $LB_CSI_JWT_PATH env
	// var, or `--jwt-path` cmd-line arg, e.g. seeded from a K8s Secret) -
	// this can't work at all...
	//
	// NOTE: this is orthogonal to the cluster-level vs project-specific
	// credentials discussed in the comment below; whether the JWT is
	// specified globally or not doesn't affect the scope of the
	// permissions granted by the JWT and vice versa.
	ctx = d.cloneCtxWithCreds(ctx, map[string]string{})
	clnt, err := d.GetLBClient(ctx, params.mgmtEPs, params.mgmtScheme)
	if err != nil {
		return nil, err
	}
	defer d.PutLBClient(clnt)

	// TODO: several issues here:
	// * GetCluster() requires cluster-level access permissions, which
	//   the caller might not have if it has project-level credentials.
	// * if `project-name` was specified, we should start using GetProject()
	//   once quotas are in place, in which case the scope of the creds
	//   won't matter.
	// * if `project-name` was NOT specified, we can probably continue using
	//   GetCluster() and just return an authZ error if the caller didn't
	//   have cluster-wide perms.
	cluster, err := clnt.GetCluster(ctx)
	if err != nil {
		return nil, err
	}

	return &csi.GetCapacityResponse{
		AvailableCapacity: int64(cluster.Capacity),
	}, nil
}

func (d *Driver) ControllerExpandVolume(
	ctx context.Context, req *csi.ControllerExpandVolumeRequest,
) (*csi.ControllerExpandVolumeResponse, error) {
	vid, err := parseCSIResourceIDEinval(volIDField, req.VolumeId)
	if err != nil {
		return nil, err
	}

	log := d.log.WithFields(logrus.Fields{
		"op":       "ControllerExpandVolume",
		"mgmt-ep":  vid.mgmtEPs,
		"vol-uuid": vid.uuid,
		"project":  vid.projName,
	})

	requestedCapacity, err := getReqCapacity(req.CapacityRange)
	if err != nil {
		return nil, err
	}

	ctx = d.cloneCtxWithCreds(ctx, req.Secrets)
	clnt, err := d.GetLBClient(ctx, vid.mgmtEPs, vid.scheme)
	if err != nil {
		return nil, err
	}
	defer d.PutLBClient(clnt)

	expandVolumeHook := func(vol *lb.Volume) (*lb.VolumeUpdate, error) {
		log = log.WithFields(logrus.Fields{
			"capacity-curr":      fmt.Sprintf("%d", vol.Capacity),
			"capacity-requested": fmt.Sprintf("%d", requestedCapacity),
		})
		// If a volume corresponding to the specified volume ID
		// is already larger than or equal to the target capacity of the expansion request,
		// the plugin SHOULD reply 0 OK.
		if requestedCapacity <= vol.Capacity {
			log.Infof("no further volume expand required")
			return nil, nil
		}
		return &lb.VolumeUpdate{Capacity: requestedCapacity}, nil
	}

	vol, err := clnt.UpdateVolume(ctx, vid.uuid, vid.projName, expandVolumeHook)
	if err != nil {
		return nil, err
	}
	if vol.Capacity < requestedCapacity {
		log.WithFields(logrus.Fields{
			"capacity-exp": fmt.Sprintf("%d", requestedCapacity),
			"capacity-got": fmt.Sprintf("%d", vol.Capacity),
		}).Errorf("clnt.UpdateVolume() succeeded, but resultant volume Capacity smaller then requested capacity")
		return nil, mkEagain("failed to expand volume %q", vid.uuid)
	}
	nodeExpansionRequired := d.nodeExpansionRequired(req.VolumeCapability)
	log.Infof("nodeExpansionRequired: %t. req.VolumeCapability: %+v", nodeExpansionRequired, req.VolumeCapability)
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         int64(vol.Capacity),
		NodeExpansionRequired: nodeExpansionRequired,
	}, nil
}

func doCreateSnapshot(
	ctx context.Context, log *logrus.Entry, clnt lb.Client, name string, srcVid lbResourceID,
	descr string,
) (*lb.Snapshot, error) {
	// check if a matching snapshot already exists (likely a result of retry from CO):
	snap, err := clnt.GetSnapshotByName(ctx, name, srcVid.projName)
	if err != nil && !isStatusNotFound(err) {
		return nil, mungeLBErr(log, err,
			"failed to check if snapshot '%s' already exists on LB", name)
	}

	if snap != nil {
		log = log.WithField("snap-uuid", snap.UUID)

		if snap.SrcVolUUID != srcVid.uuid {
			return nil, mkEExist("snapshot '%s' already exists but is incompatible: "+
				"it is based on volume %s instead of the requested one: %s",
				name, snap.SrcVolUUID, srcVid.uuid)
		}

		switch snap.State {
		case lb.SnapshotAvailable:
			// appears to be usable as is...
		case lb.SnapshotCreating:
			// NOTE: we can't return this snap even with `ready_to_use` set
			// to `false`. the CSI spec allows to do this only if the snap
			// was successfully cut, with only the immediate snapshot
			// usability being at stake - but not its existence!. in LB case
			// it is not guaranteed that a snapshot will transition from
			// 'Creating' to 'Available'. so, just trigger a retry explicitly
			// letting the CO decide what it wants to do with the workload.
			return nil, mkEagain("snapshot '%s' is still being created", name)
		case lb.SnapshotDeleting:
			// TODO: currently this is a lose-lose situation: it will be
			// impossible to create a new snapshot with name `name` until
			// the previous one is gone, and it will be impossible to use
			// the previous one since it's in the 'Deleting' state. worse,
			// currently CreateSnapshot() calls colliding with such zombie
			// snapshots will SUCCEED instead of returning some error (e.g.
			// 'FailedPrecondition', or 'AlreadyExists'), but the resultant
			// snapshot will be created in the 'Failed' state, unusable...
			//
			// worse still, the current naming of "intermediate snapshots"
			// used on volume clone flows ties the snapshot name to the
			// target clone volume name, so it'll be impossible to us the
			// LB CSI plugin for create->delete->create clone flows with a
			// given name except if pausing for a very long time before
			// the 2nd create, as snapshot deletion is a long-running
			// background operation. see more on this in CreateVolume()...

			// per CSI spec, this SHOULD cause the CO to retry with exp.
			// backoff. this might - or might not - help, depending on the
			// reason the volume ended up being 'Deleting', and lets the
			// clone flow of CreateVolume() know what's going on:
			return nil, mkAbort("snapshot '%s' is in the process of being deleted, "+
				"try again later", name)
		default:
			return nil, mkInternal("snapshot '%s' already exists but is in unexpected "+
				"state '%s' (%d)", name, snap.State, snap.State)
		}

		log.Info("snapshot already exists")
		return snap, nil
	}

	// ...nope, need to actually create a new snapshot:
	snap, err = clnt.CreateSnapshot(ctx, name, srcVid.projName, srcVid.uuid, descr, true)
	if err != nil {
		return nil, mungeLBErr(log, err, "failed to create snapshot '%s'", name)
	}

	log.WithField("snap-uuid", snap.UUID).Info("snapshot created successfully")
	return snap, nil
}

func (d *Driver) CreateSnapshot(
	ctx context.Context, req *csi.CreateSnapshotRequest,
) (*csi.CreateSnapshotResponse, error) {
	srcVid, err := parseCSIResourceIDEinval(srcVolField, req.SourceVolumeId)
	if err != nil {
		return nil, err
	}

	log := d.log.WithFields(logrus.Fields{
		"op":           "CreateSnapshot",
		"mgmt-ep":      srcVid.mgmtEPs,
		"snap-name":    req.Name,
		"src-vol-uuid": srcVid.uuid,
		"project":      srcVid.projName,
	})

	// TODO: initially the LB CSI plugin supports no custom `req.parameters`
	// entries. if it becomes necessary, their parsing should be added HERE.

	ctx = d.cloneCtxWithCreds(ctx, req.Secrets)
	clnt, err := d.GetLBClient(ctx, srcVid.mgmtEPs, srcVid.scheme)
	if err != nil {
		return nil, err
	}
	defer d.PutLBClient(clnt)

	snap, err := doCreateSnapshot(ctx, log, clnt, req.Name, srcVid, "by: LB CSI")
	if err != nil {
		return nil, err
	}

	snapID := lbResourceID{
		mgmtEPs:  srcVid.mgmtEPs,
		uuid:     snap.UUID,
		projName: snap.ProjectName,
		scheme:   srcVid.scheme,
	}
	return &csi.CreateSnapshotResponse{
		Snapshot: &csi.Snapshot{
			SnapshotId:     snapID.String(),
			SourceVolumeId: srcVid.String(),
			SizeBytes:      int64(snap.Capacity),
			CreationTime:   timestamppb.New(snap.CreationTime),
			ReadyToUse:     true, // see note in doCreateSnapshot()
		},
	}, nil
}

func doDeleteSnapshot(
	ctx context.Context, log *logrus.Entry, clnt lb.Client, sid lbResourceID,
) error {
	snap, err := clnt.GetSnapshot(ctx, sid.uuid, sid.projName)
	if err != nil {
		if isStatusNotFound(err) {
			log.Info("snapshot already gone")
			return nil
		}
		return mungeLBErr(log, err, "failed to get snapshot %s from LB", sid.uuid)
	}

	switch snap.State {
	case lb.SnapshotAvailable:
		// this is really the only one that can and should be deleted.
	case lb.SnapshotDeleting,
		lb.SnapshotFailed:
		log.Info("snapshot effectively already gone")
		return nil
	case lb.SnapshotCreating:
		return mkEagain("snapshot %s is still being created", snap.UUID)
	default:
		return mkInternal("found snapshot '%s' (%s) in unexpected state '%s' (%d)",
			snap.Name, snap.UUID, snap.State, snap.State)
	}

	err = clnt.DeleteSnapshot(ctx, sid.uuid, sid.projName, true)
	if err != nil {
		return mungeLBErr(log, err, "failed to delete snapshot %s from LB", sid.uuid)
	}

	log.Info("snapshot deleted")
	return nil
}

func (d *Driver) DeleteSnapshot(
	ctx context.Context, req *csi.DeleteSnapshotRequest,
) (*csi.DeleteSnapshotResponse, error) {
	log := d.log.WithField("op", "DeleteSnapshot")
	sid, err := parseCSIResourceIDEnoent(snapIDField, req.SnapshotId)
	if err != nil {
		if isStatusNotFound(err) {
			log.Errorf("bad value of '%s': %s", snapIDField, err)
			// returning success instead of error to pacify some of the more
			// simple-minded external tests:
			return &csi.DeleteSnapshotResponse{}, nil
		}
		return nil, err
	}

	log = d.log.WithFields(logrus.Fields{
		"mgmt-ep":   sid.mgmtEPs,
		"snap-uuid": sid.uuid,
		"project":   sid.projName,
	})

	ctx = d.cloneCtxWithCreds(ctx, req.Secrets)
	clnt, err := d.GetLBClient(ctx, sid.mgmtEPs, sid.scheme)
	if err != nil {
		return nil, err
	}
	defer d.PutLBClient(clnt)

	err = doDeleteSnapshot(ctx, log, clnt, sid)
	if err != nil {
		return nil, err
	}
	return &csi.DeleteSnapshotResponse{}, nil
}

func (d *Driver) ListSnapshots(
	ctx context.Context, req *csi.ListSnapshotsRequest,
) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
