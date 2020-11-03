// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/lightbitslabs/lb-csi/pkg/lb"
	"github.com/lightbitslabs/lb-csi/pkg/util/endpoint"
	"github.com/lightbitslabs/lb-csi/pkg/util/strlist"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	ctx context.Context, req *csi.ControllerGetCapabilitiesRequest,
) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: capsCache,
	}, nil
}

func getReqCapacity(capRange *csi.CapacityRange) (uint64, error) {
	// potentially those two might be cluster-specific in the future, so we'd
	// need to grab them using the LightOS mgmt API first:
	volCap := minVolCap
	capGran := volCapGranularity

	if capRange == nil {
		return uint64(volCap), nil
	}

	// TODO: AFAICT, should return "11 OUT_OF_RANGE" error instead of EINVAL

	minCap := capRange.RequiredBytes
	maxCap := capRange.LimitBytes
	if minCap < 0 || maxCap < 0 {
		return 0, mkEinvalf("capacity_range",
			"invalid range specified: [%d..%d]", minCap, maxCap)
	}
	if minCap == 0 && maxCap == 0 {
		return 0, mkEinvalf("capacity_range",
			"both 'required_bytes' and 'limit_bytes' are missing")
	}
	if maxCap != 0 && maxCap < minVolCap {
		return 0, mkEinvalf("capacity_range.limit_bytes",
			"minimum supported volume size: %d", minVolCap)
	}
	if minCap != 0 && maxCap != 0 && maxCap < minCap {
		return 0, mkEinvalf("capacity_range",
			"invalid range specified: [%d..%d]", minCap, maxCap)
	}
	volCap = (minCap + capGran - 1) / capGran * capGran
	if maxCap != 0 && volCap > maxCap {
		return 0, mkEinvalf("capacity_range",
			"capacity granularity is %d bytes, can't create volume of "+
				"capacity range: [%d..%d]", capGran, minCap, maxCap)
	}

	return uint64(volCap), nil
}

func mkVolumeResponse(mgmtEPs endpoint.Slice, vol *lb.Volume) *csi.CreateVolumeResponse {
	volID := lbVolumeID{
		mgmtEPs: mgmtEPs,
		uuid:    vol.UUID,
	}
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			CapacityBytes: int64(vol.Capacity),
			VolumeId:      volID.String(),
		},
	}
}

func (d *Driver) doCreateVolume(
	ctx context.Context, mgmtEPs endpoint.Slice, req lb.Volume,
) (*csi.CreateVolumeResponse, error) {
	log := d.log.WithFields(logrus.Fields{
		"op":       "CreateVolume",
		"mgmt-ep":  mgmtEPs,
		"vol-name": req.Name,
	})

	clnt, err := d.GetLBClient(ctx, mgmtEPs)
	if err != nil {
		return nil, err
	}
	defer d.PutLBClient(clnt)

	// check if a matching volume already exists (likely a result of retry from CO):
	vol, err := clnt.GetVolumeByName(ctx, req.Name)
	if err != nil && !isStatusNotFound(err) {
		// TODO: convert to status!
		return nil, err
	}
	if err == nil {
		switch vol.State {
		case lb.VolumeAvailable,
			lb.VolumeUpdating:
			// this one might be reusable as is...
		case lb.VolumeCreating:
			return nil, mkEagain("volume '%s' is still being created", vol.Name)
		default:
			return nil, mkInternal("volume '%s' already exists but is in unexpected "+
				"state '%s' (%d)", vol.Name, vol.State, vol.State)
		}

		log = log.WithField("vol-uuid", vol.UUID)

		// TODO: check protection state and stall with EAGAIN, making
		// the CO retry, if it's read-only on unavailable?
		// this way the user workload will be DELAYED, possibly for a
		// long time - until the relevant LightOS cluster nodes are up
		// and caught up - but at least will not see EIO right off the
		// bat (that might still happen on massive LightOS cluster node
		// outages after the volume is created and returned, of course)...

		diffs := req.ExplainDiffsFrom(vol, "requested", "actual", true)
		if len(diffs) > 0 {
			return nil, mkEExist("volume '%s' already exists but is incompatible: %s",
				vol.Name, strings.Join(diffs, ", "))
		}
		if !strlist.AreEqual(vol.ACL, req.ACL) {
			// this is likely a race with some other instance, but
			// the ACL should be properly adjusted afterwards, on
			// ControllerPublishVolume()/ControllerUnpublishVolume()
			// (which the other instance may have already reached).
			log.Warnf("found matching existing volume with "+
				"unexpected ACL %#q instead of %#q", vol.ACL, req.ACL)
		}

		// if reached here - a matching volume already exists...
		if !vol.IsWritable() {
			log.Warnf("volume already exists, but is not currently usable: "+
				"its protection state is '%s'", vol.Protection)
		} else {
			log.Info("volume already exists")
		}
	} else {
		// nope, no such luck. just create one:
		vol, err = clnt.CreateVolume(ctx, req.Name, req.Capacity, req.ReplicaCount,
			req.Compression, req.ACL, true)
		if err != nil {
			// TODO: convert to status!
			return nil, err
		}
		log.WithField("vol-uuid", vol.UUID).Info("volume created")
	}

	return mkVolumeResponse(mgmtEPs, vol), nil
}

// CreateVolume uses info extracted from request `parameters` field to connect
// to LB and attempt to create the volume specified by `name` field (or return
// info on an existing volume if one matches, for idempotency). see
// `lbCreateVolumeParams` for more details on the format.
func (d *Driver) CreateVolume(
	ctx context.Context, req *csi.CreateVolumeRequest,
) (*csi.CreateVolumeResponse, error) {
	if req.Name == "" {
		return nil, mkEinvalMissing("name")
	}

	if req.VolumeContentSource != nil {
		return nil, mkEinval("volume_content_source",
			"volumes prepopulation is not supported")
	}

	if req.AccessibilityRequirements != nil {
		return nil, mkEinval("accessibility_requirements",
			"accessibility constraints are not supported")
	}

	capacity, err := getReqCapacity(req.CapacityRange)
	if err != nil {
		return nil, err
	}

	if err = d.validateVolumeCapabilities(req.VolumeCapabilities); err != nil {
		return nil, err
	}

	params, err := ParseCSICreateVolumeParams(req.Parameters)
	if err != nil {
		return nil, err
	}

	vol, err := d.doCreateVolume(ctx, params.mgmtEPs, lb.Volume{
		Name:         req.Name,
		Capacity:     capacity,
		ReplicaCount: params.replicaCount,
		Compression:  params.compression,
		ACL:          []string{lb.ACLAllowNone},
	})
	if err != nil {
		return nil, err
	}

	return vol, nil
}

func (d *Driver) DeleteVolume(
	ctx context.Context, req *csi.DeleteVolumeRequest,
) (*csi.DeleteVolumeResponse, error) {
	vid, err := ParseCSIVolumeID(req.VolumeId)
	if err != nil {
		if errors.Is(err, ErrMalformed) {
			d.log.WithFields(logrus.Fields{
				"op":     "DeleteVolume",
				"vol-id": req.VolumeId,
			}).WithError(err).Errorf("req.volumeId not valid. returning success according to spec")
			return &csi.DeleteVolumeResponse{}, nil
		}
		return nil, mkEinval("volume_id", err.Error())
	}

	log := d.log.WithFields(logrus.Fields{
		"op":       "DeleteVolume",
		"mgmt-ep":  vid.mgmtEPs,
		"vol-uuid": vid.uuid,
	})

	clnt, err := d.GetLBClient(ctx, vid.mgmtEPs)
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
	vol, err := clnt.GetVolume(ctx, vid.uuid)
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
			vol.Name, vol.UUID.String(), vol.State, vol.State)
	}

	// TODO: consider checking if the volume is in use at this time? e.g.
	// using connectedHosts field of the volume? it's not exactly exact
	// science, what with TOCTTOU, and all, but still might avoid accidental
	// user data loss...

	// oh, well, just delete it:
	err = clnt.DeleteVolume(ctx, vol.UUID, true)
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
	vid, err := ParseCSIVolumeID(req.VolumeId)
	if err != nil {
		return nil, mkEinval("volume_id", err.Error())
	}

	log := d.log.WithFields(logrus.Fields{
		"op":       "ControllerPublishVolume",
		"mgmt-ep":  vid.mgmtEPs,
		"vol-uuid": vid.uuid,
	})

	if err := checkNodeID(req.NodeId); err != nil {
		// NodeId should have been previously validated by the node
		// instance of the driver upon start-up and returned from
		// NodeGetInfo(), so this is odd...
		d.log.Errorf("unexpected node_id: '%s'", req.NodeId)
		return nil, mkEinval("node_id", err.Error())
	}
	// TODO: this one is tricky, because the CSI plugin is supposed to be
	// able to figure out if the volume has previously been published to
	// the node in question with an incompatible `volume_capability` (e.g.
	// as block volume instead of FS mount, presumably?). except how would
	// THIS instance know about what went on a long time ago on a node
	// far, far away?
	if err = d.validateVolumeCapability(req.VolumeCapability); err != nil {
		return nil, err
	}
	if req.Readonly {
		return nil, mkEinval("readonly", "read-only volumes are not supported")
	}

	log = log.WithField("node-id", req.NodeId)
	ace := nodeIDToHostNQN(req.NodeId)

	clnt, err := d.GetLBClient(ctx, vid.mgmtEPs)
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

	vol, err := clnt.UpdateVolume(ctx, vid.uuid, publishVolumeHook)
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
	vid, err := ParseCSIVolumeID(req.VolumeId)
	if err != nil {
		return nil, mkEinval("volume_id", err.Error())
	}

	log := d.log.WithFields(logrus.Fields{
		"op":       "ControllerUnpublishVolume",
		"mgmt-ep":  vid.mgmtEPs,
		"vol-uuid": vid.uuid,
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

	clnt, err := d.GetLBClient(ctx, vid.mgmtEPs)
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

	vol, err := clnt.UpdateVolume(ctx, vid.uuid, unpublishVolumeHook)
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
	if req.VolumeId == "" {
		return nil, mkEinvalMissing("volume_id")
	}
	if req.VolumeCapabilities == nil {
		return nil, mkEinvalMissing("volume_capabilities")
	}

	vid, err := ParseCSIVolumeID(req.VolumeId)
	if err != nil {
		if errors.Is(err, ErrMalformed) {
			d.log.WithFields(logrus.Fields{
				"op":     "ValidateVolumeCapabilities",
				"vol-id": req.VolumeId,
			}).WithError(err).Errorf("req.volumeId not valid. returning success according to spec")
			return nil, err
		}
		return nil, mkEinval("volume_id", err.Error())
	}

	log := d.log.WithFields(logrus.Fields{
		"op":       "ValidateVolumeCapabilities",
		"mgmt-ep":  vid.mgmtEPs,
		"vol-uuid": vid.uuid,
	})

	clnt, err := d.GetLBClient(ctx, vid.mgmtEPs)
	if err != nil {
		return nil, err
	}
	defer d.PutLBClient(clnt)

	if _, err := clnt.GetVolume(ctx, vid.uuid); err != nil {
		if isStatusNotFound(err) {
			return nil, mkEnoent("volume '%s' doesn't exist", vid)
		}
		return nil, d.mungeLBErr(log, err, "failed to get volume '%s' from LB", vid)
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
	ctx context.Context, req *csi.ListVolumesRequest,
) (*csi.ListVolumesResponse, error) {

	// TODO: er... impl?

	return nil, status.Error(codes.Unimplemented, "")
}

func (d *Driver) GetCapacity(
	ctx context.Context, req *csi.GetCapacityRequest,
) (*csi.GetCapacityResponse, error) {

	// TODO: er... impl?

	return nil, status.Error(codes.Unimplemented, "")
}

func (d *Driver) ControllerExpandVolume(
	ctx context.Context, req *csi.ControllerExpandVolumeRequest,
) (*csi.ControllerExpandVolumeResponse, error) {

	vid, err := ParseCSIVolumeID(req.VolumeId)
	if err != nil {
		return nil, mkEinval("volume_id", err.Error())
	}

	log := d.log.WithFields(logrus.Fields{
		"op":       "ControllerExpandVolume",
		"mgmt-ep":  vid.mgmtEPs,
		"vol-uuid": vid.uuid,
	})

	requestedCapacity, err := getReqCapacity(req.CapacityRange)
	if err != nil {
		return nil, err
	}

	clnt, err := d.GetLBClient(ctx, vid.mgmtEPs)
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

	vol, err := clnt.UpdateVolume(ctx, vid.uuid, expandVolumeHook)
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
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         int64(vol.Capacity),
		NodeExpansionRequired: d.nodeExpansionRequired(req.VolumeCapability),
	}, nil
}

// --------------------------------------------------------------------------

// graveyard: support for the below not planned...

func (d *Driver) CreateSnapshot(
	ctx context.Context, req *csi.CreateSnapshotRequest,
) (*csi.CreateSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (d *Driver) DeleteSnapshot(
	ctx context.Context, req *csi.DeleteSnapshotRequest,
) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (d *Driver) ListSnapshots(
	ctx context.Context, req *csi.ListSnapshotsRequest,
) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
