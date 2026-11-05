// Copyright (C) 2016--2026 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
		if va.Spec.Attacher != driverName || va.DeletionTimestamp != nil {
			continue
		}
		vaNodeID, err := a.vaNodeID(ctx, va.Name, va.Annotations, va.Spec.NodeName)
		if err != nil {
			return nil, err
		}
		if vaNodeID != nodeID {
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

// vaNodeID resolves the CSI node ID of an attachment the way external-attacher
// does: its annotation, else the node's CSINode. an attachment it cannot place
// is an error, so the caller keeps the trusted host instead of taking the
// node for idle.
func (a *k8sNodeAttachments) vaNodeID(
	ctx context.Context, vaName string, annotations map[string]string, nodeName string,
) (string, error) {
	if id, ok := annotations[vaNodeIDAnnotation]; ok {
		return id, nil
	}
	csiNode, err := a.cs.StorageV1().CSINodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to resolve node of VolumeAttachment %s: %w", vaName, err)
	}
	for _, drv := range csiNode.Spec.Drivers {
		if drv.Name == driverName {
			return drv.NodeID, nil
		}
	}
	return "", fmt.Errorf("failed to resolve node of VolumeAttachment %s: "+
		"CSINode %s has no %s entry", vaName, nodeName, driverName)
}

// nodeLocks hands out one lock per node ID. entries are never freed: there
// is one per node that ever had a volume published to it.
type nodeLocks struct {
	mu    sync.Mutex
	locks map[string]*sync.RWMutex
}

func (l *nodeLocks) get(nodeID string) *sync.RWMutex {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.locks == nil {
		l.locks = make(map[string]*sync.RWMutex)
	}
	lock, ok := l.locks[nodeID]
	if !ok {
		lock = &sync.RWMutex{}
		l.locks[nodeID] = lock
	}
	return lock
}

// nodeAttachments returns nil unless in-band auth is enabled and the plugin
// runs in a K8s cluster. it is built on first use, so only the plugin
// instance that serves ControllerUnpublishVolume builds it.
func (d *Driver) nodeAttachments() nodeAttachments {
	d.attachmentsOnce.Do(func() {
		if !d.inBandAuth {
			return
		}
		att, err := newK8sNodeAttachments()
		if err != nil {
			d.log.WithError(err).Warn("trusted host cleanup on detach disabled")
			return
		}
		d.attachments = att
	})
	return d.attachments
}

// reapTrustedHost removes the in-band auth trusted host of `nodeID` from the
// LB cluster serving `vid` once no other volume of that cluster and project
// is attached to the node, revoking the node secret pair when the node stops
// using the cluster. a node's trusted host serves a single project, so
// volumes of other projects never hold it.
//
// it holds the node lock for writing from the attachment list to the delete.
// ControllerPublishVolume holds it for reading, and the attacher annotates
// its VolumeAttachment before it publishes, so a concurrent attach is either
// listed here or finds the trusted host gone and NodeStageVolume re-creates
// it.
//
// a transient error is returned, so the CO retries the detach and with it the
// cleanup. any other failure keeps the trusted host and is only logged, so a
// detach never blocks on cleanup for good.
func (d *Driver) reapTrustedHost(
	ctx context.Context, clnt lb.Client, log *logrus.Entry, vid lbResourceID, nodeID string,
) error {
	att := d.nodeAttachments()
	if att == nil {
		return nil
	}
	log = log.WithField("trusted-host", nodeID)

	cictx, cancel := context.WithTimeout(ctx, 10*time.Second)
	ci, err := clnt.GetClusterInfo(cictx)
	cancel()
	if err != nil {
		return reapFailed(log, err, "failed to get cluster info")
	}
	if !ci.InBandAuthEnabled {
		return nil
	}

	lock := d.nodeLocks.get(nodeID)
	lock.Lock()
	defer lock.Unlock()

	ids, err := att.AttachedVolumeIDs(ctx, nodeID)
	if err != nil {
		if isTransientK8sErr(err) {
			log.WithError(err).Warn("trusted host cleanup deferred: failed to list node attachments")
			return mkEagain("temporarily failed to list the attachments of node '%s'", nodeID)
		}
		log.WithError(err).Warn("trusted host cleanup skipped: failed to list node attachments")
		return nil
	}
	for _, id := range ids {
		vlog := log.WithField("attached-vol", id)
		other, err := parseCSIResourceID(id)
		if err != nil {
			vlog.WithError(err).Warn("trusted host cleanup skipped: unparsable volume ID")
			return nil
		}
		if other.projName != vid.projName || other.uuid == vid.uuid {
			continue
		}
		// volume UUIDs are unique, so finding the volume on this cluster is
		// what places it here, whatever mgmt endpoints its StorageClass lists.
		_, err = clnt.GetVolume(ctx, other.uuid, other.projName)
		if err == nil {
			vlog.Debug("trusted host still in use")
			return nil
		}
		if !isStatusNotFound(err) {
			return reapFailed(vlog, err, "failed to look up attached volume")
		}
	}

	err = clnt.DeleteTrustedHost(ctx, nodeID, vid.projName)
	if err != nil && !isStatusNotFound(err) {
		return reapFailed(log, err, "failed to delete trusted host")
	}
	log.Info("trusted host removed: no volumes of this cluster and project left attached")
	return nil
}

func reapFailed(log *logrus.Entry, err error, what string) error {
	if shouldRetryOn(err) {
		return mungeLBErr(log, err, "trusted host cleanup: %s", what)
	}
	log.WithError(err).Warnf("trusted host cleanup skipped: %s", what)
	return nil
}

func isTransientK8sErr(err error) bool {
	return apierrors.IsServerTimeout(err) || apierrors.IsTimeout(err) ||
		apierrors.IsTooManyRequests(err) || apierrors.IsServiceUnavailable(err) ||
		apierrors.IsInternalError(err)
}
