// Copyright (C) 2016--2026 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/lightbitslabs/los-csi/pkg/lb"
)

// external-attacher records the CSI node ID on a VolumeAttachment before it
// calls ControllerPublishVolume, and keeps it after the Node object is gone.
const vaNodeIDAnnotation = "csi.alpha.kubernetes.io/node-id"

// nodeAttachments lists the CSI volume IDs the CO still holds attached to a
// node through this driver, excluding attachments already being detached:
// the CO marks one for deletion only once the node stopped using the volume,
// or once it gave up on the node.
type nodeAttachments interface {
	AttachedVolumeIDs(ctx context.Context, nodeID string) ([]string, error)
}

type k8sNodeAttachments struct {
	cs kubernetes.Interface
}

func newK8sNodeAttachments() (nodeAttachments, error) {
	kcfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	cs, err := kubernetes.NewForConfig(kcfg)
	if err != nil {
		return nil, err
	}
	return &k8sNodeAttachments{cs: cs}, nil
}

func (a *k8sNodeAttachments) AttachedVolumeIDs(
	ctx context.Context, nodeID string,
) ([]string, error) {
	vas, err := a.cs.StorageV1().VolumeAttachments().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list VolumeAttachments: %w", err)
	}
	var ids []string
	for i := range vas.Items {
		va := &vas.Items[i]
		if va.Spec.Attacher != driverName || va.DeletionTimestamp != nil ||
			va.Annotations[vaNodeIDAnnotation] != nodeID {
			continue
		}
		src := va.Spec.Source
		if src.InlineVolumeSpec != nil && src.InlineVolumeSpec.CSI != nil {
			ids = append(ids, src.InlineVolumeSpec.CSI.VolumeHandle)
			continue
		}
		if src.PersistentVolumeName == nil {
			return nil, fmt.Errorf("VolumeAttachment %s has no volume source", va.Name)
		}
		pv, err := a.cs.CoreV1().PersistentVolumes().Get(
			ctx, *src.PersistentVolumeName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get PV %s: %w", *src.PersistentVolumeName, err)
		}
		if pv.Spec.CSI == nil {
			return nil, fmt.Errorf("PV %s is not a CSI volume", pv.Name)
		}
		ids = append(ids, pv.Spec.CSI.VolumeHandle)
	}
	return ids, nil
}

// reapTrustedHost removes the in-band auth trusted host of `nodeID` from the
// LB cluster serving `vid` once no other volume of that cluster and project
// is attached to the node, revoking the node secret pair when the node stops
// using the cluster. a node's trusted host serves a single project, so
// volumes of other projects never hold it.
//
// the caller must hold the node lock across the volume ACL update and this
// call: ControllerPublishVolume takes the same lock after the attacher has
// annotated its VolumeAttachment, so a concurrent attach is either listed
// here or finds the trusted host gone and NodeStageVolume re-creates it.
//
// failures are logged, not returned: a detach must not block on cleanup.
func (d *Driver) reapTrustedHost(
	ctx context.Context, clnt lb.Client, log *logrus.Entry, vid lbResourceID, nodeID string,
) {
	if d.attachments == nil {
		return
	}
	log = log.WithField("trusted-host", nodeID)

	cictx, cancel := context.WithTimeout(ctx, 10*time.Second)
	ci, err := clnt.GetClusterInfo(cictx)
	cancel()
	if err != nil {
		log.WithError(err).Warn("trusted host cleanup skipped: failed to get cluster info")
		return
	}
	if !ci.InBandAuthEnabled {
		return
	}

	ids, err := d.attachments.AttachedVolumeIDs(ctx, nodeID)
	if err != nil {
		log.WithError(err).Warn("trusted host cleanup skipped: failed to list node attachments")
		return
	}
	for _, id := range ids {
		vlog := log.WithField("attached-vol", id)
		other, err := parseCSIResourceID(id)
		if err != nil {
			vlog.WithError(err).Warn("trusted host cleanup skipped: unparsable volume ID")
			return
		}
		if other.projName != vid.projName || other.uuid == vid.uuid {
			continue
		}
		// volume UUIDs are unique, so finding the volume on this cluster is
		// what places it here, whatever mgmt endpoints its StorageClass lists.
		_, err = clnt.GetVolume(ctx, other.uuid, other.projName)
		if err == nil {
			vlog.Debug("trusted host still in use")
			return
		}
		if !isStatusNotFound(err) {
			vlog.WithError(err).Warn("trusted host cleanup skipped: failed to look up attached volume")
			return
		}
	}

	err = clnt.DeleteTrustedHost(ctx, nodeID, vid.projName)
	if err != nil && !isStatusNotFound(err) {
		log.WithError(err).Warn("trusted host cleanup failed")
		return
	}
	log.Info("trusted host removed: no volumes of this cluster left attached to the node")
}
