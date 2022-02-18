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

func mkVolumeResponse(mgmtEPs endpoint.Slice, vol *lb.Volume, mgmtScheme string) *csi.CreateVolumeResponse {
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
		},
	}
}

func (d *Driver) validateSnapshot(ctx context.Context, clnt lb.Client, req lb.Volume) (bool, error) {
	if req.SnapshotUUID == guuid.Nil {
		return true, nil
	}

	lbSnap, err := clnt.GetSnapshot(ctx, req.SnapshotUUID, req.ProjectName)
	if err != nil {
		return false, mkInternal("unable to get snapshot %s", req.SnapshotUUID.String())
	}

	switch lbSnap.State {
	case lb.SnapshotAvailable:
		// this is the only one that should permit volume creation.
	case lb.SnapshotDeleting,
		lb.SnapshotFailed:
		return false, mkInternal("snapshot '%s' (%s) is in state '%s' (%d)",
			lbSnap.Name, lbSnap.UUID.String(), lbSnap.State, lbSnap.State)
	case lb.SnapshotCreating:
		return false, mkEagain("snapshot %s is still being created", lbSnap.UUID)
	default:
		return false, mkInternal("found snapshot '%s' (%s) in unexpected state '%s' (%d)",
			lbSnap.Name, lbSnap.UUID.String(), lbSnap.State, lbSnap.State)
	}

	if req.ReplicaCount != lbSnap.SrcVolReplicaCount {
		return false, mkEinvalf("volume %s requested replicaCount %d != snapshot replicaCount %d",
			req.Name, req.ReplicaCount, lbSnap.SrcVolReplicaCount)
	}
	if req.Compression != lbSnap.SrcVolCompression {
		return false, mkEinvalf("volume %s requested compression %d != snapshot compression %d",
			req.Name, req.Compression, lbSnap.SrcVolCompression)
	}
	if req.Capacity < lbSnap.Capacity {
		return false, mkEinvalf("volume %s requested size %d < snapshot %s size %d",
			req.Name, req.Capacity, lbSnap.Capacity)
	}

	return true, nil
}

func (d *Driver) validateVolume(ctx context.Context, req lb.Volume, vol *lb.Volume) (bool, error) {
	log := d.log.WithFields(logrus.Fields{
		"op":       "validateVolume",
		"vol-name": vol.Name,
		"vol-uuid": vol.UUID,
	})
	switch vol.State {
	case lb.VolumeAvailable, lb.VolumeUpdating:
		// this one might be reusable as is...
	case lb.VolumeCreating:
		return false, mkEagain("volume '%s' is still being created", vol.Name)
	default:
		return false, mkInternal("volume '%s' exists but is in unexpected "+
			"state '%s' (%d)", vol.Name, vol.State, vol.State)
	}

	// TODO: check protection state and stall with EAGAIN, making
	// the CO retry, if it's read-only on unavailable?
	// this way the user workload will be DELAYED, possibly for a
	// long time - until the relevant LightOS cluster nodes are up
	// and caught up - but at least will not see EIO right off the
	// bat (that might still happen on massive LightOS cluster node
	// outages after the volume is created and returned, of course)...

	diffs := req.ExplainDiffsFrom(vol, "requested", "actual", lb.SkipUUID)
	if len(diffs) > 0 {
		return false, mkEExist("volume '%s' exists but is incompatible: %s",
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

	return true, nil
}

func (d *Driver) doCreateVolume(
	ctx context.Context, mgmtScheme string, mgmtEPs endpoint.Slice, req lb.Volume,
) (*csi.CreateVolumeResponse, error) {
	log := d.log.WithFields(logrus.Fields{
		"op":       "CreateVolume",
		"mgmt-ep":  mgmtEPs,
		"vol-name": req.Name,
		"project":  req.ProjectName,
	})

	clnt, err := d.GetLBClient(ctx, mgmtEPs, mgmtScheme)
	if err != nil {
		return nil, err
	}
	defer d.PutLBClient(clnt)

	// check if a matching volume already exists (likely a result of retry from CO):
	vol, err := clnt.GetVolumeByName(ctx, req.Name, req.ProjectName)
	if err != nil {
		if !isStatusNotFound(err) {
			// TODO: convert to status!
			return nil, err
		}
		// nope, no such luck. just create one:
		if ok, err := d.validateSnapshot(ctx, clnt, req); !ok {
			return nil, err
		}

		vol, err = clnt.CreateVolume(ctx, req.Name, req.Capacity, req.ReplicaCount,
			req.Compression, req.ACL, req.ProjectName, req.SnapshotUUID, true) // TODO: blocking opt
		if err != nil {
			// TODO: convert to status!
			return nil, err
		}

	}

	// verify what we asked for is what we got...
	if ok, err := d.validateVolume(ctx, req, vol); !ok {
		return nil, err
	}

	log.WithField("vol-uuid", vol.UUID).Info("volume created successfully")
	return mkVolumeResponse(mgmtEPs, vol, mgmtScheme), nil
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

	params, err := parseCSICreateVolumeParams(req.Parameters)
	if err != nil {
		return nil, err
	}

	log := d.log.WithFields(logrus.Fields{
		"op":      "CreateVolume",
		"mgmt-ep": params.mgmtEPs,
		"project": params.projectName,
	})

	var snapshotID string
	snapshotUUID := guuid.UUID{}
	contentSource := req.GetVolumeContentSource()
	if contentSource != nil {
		if contentSource.GetVolume() != nil {
			log.Debugf("clone volume from volume - create intermediate snapshot")
			snapReq := csi.CreateSnapshotRequest{
				SourceVolumeId: contentSource.GetVolume().GetVolumeId(),
				Name:           "snapshot-" + req.GetName(),
				Parameters:     req.GetParameters(),
				Secrets:        req.Secrets,
			}
			snap, err := d.CreateSnapshot(ctx, &snapReq)
			if err != nil {
				log.Errorf("clone volume from volume - create intermediate snapshot failed")
				return nil, err
			}
			snapshotID = snap.GetSnapshot().GetSnapshotId()
		} else {
			log.Debugf("clone volume from snapshot")
			snapshotID = contentSource.GetSnapshot().SnapshotId
		}
		sid, err := parseCSIResourceID(snapshotID)
		if err != nil {
			return nil, mkEinval("SnapshotID", err.Error())
		}
		snapshotUUID = sid.uuid
	}

	ctx = d.cloneCtxWithCreds(ctx, req.Secrets)
	vol, err := d.doCreateVolume(ctx, params.mgmtScheme, params.mgmtEPs,
		lb.Volume{
			Name:         req.Name,
			Capacity:     capacity,
			ReplicaCount: params.replicaCount,
			Compression:  params.compression,
			ACL:          []string{lb.ACLAllowNone},
			SnapshotUUID: snapshotUUID,
			ProjectName:  params.projectName,
		})
	if err != nil {
		return nil, err
	}
	if contentSource != nil {
		if contentSource.GetVolume() != nil {
			log.Debugf("clone volume from volume - delete intermediate snapshot")
			req := csi.DeleteSnapshotRequest{
				SnapshotId: snapshotID,
			}
			_, err := d.DeleteSnapshot(ctx, &req)
			if err != nil {
				log.Errorf("clone volume from volume - delete intermediate snapshot failed")
				// TODO - currently LightOS doesn't support deleting a snapshot
				// while there are cloned volumes from it. Uncomment once it does:
				// return nil, err
			}

		}
	}
	vol.Volume.ContentSource = contentSource

	return vol, nil
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
		"op":        "CreateSnapshot",
		"mgmt-ep":   srcVid.mgmtEPs,
		"snap-name": req.Name,
		"project":   srcVid.projName,
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
			CreationTime:   snap.CreationTime,
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
