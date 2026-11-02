// Copyright (C) 2016--2020 Lightbits Labs Ltd.
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
)

const k8sListAttempts = 5

// trackIPAclVolsFromK8s refills the IP-ACL re-check list at startup. kubelet
// does not stage the volumes again after a plugin restart, so the list is
// rebuilt from the volumes attached to this node, with the JWT of their
// node-stage Secret.
func (d *Driver) trackIPAclVolsFromK8s(ctx context.Context) {
	log := d.log.WithFields(logrus.Fields{"op": "IPAclRecheck", "k8s-node": d.k8sNodeName})

	cfg, err := rest.InClusterConfig()
	if err != nil {
		log.WithError(err).Warn("can't load K8s in-cluster config")
		return
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.WithError(err).Warn("can't create K8s client")
		return
	}

	for attempt := 1; ; attempt++ {
		n, err := d.loadIPAclVolsFromK8s(ctx, log, cs)
		if err == nil {
			log.WithField("volumes", n).Info("IP-ACL re-check list loaded from K8s")
			return
		}
		log.WithError(err).WithField("attempt", attempt).Warn("failed to list K8s VolumeAttachments")
		if attempt == k8sListAttempts {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(ipACLRecheckPeriod):
		}
	}
}

func (d *Driver) loadIPAclVolsFromK8s(
	ctx context.Context, log *logrus.Entry, cs kubernetes.Interface,
) (int, error) {
	vas, err := cs.StorageV1().VolumeAttachments().List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, err
	}
	n := 0
	for i := range vas.Items {
		va := &vas.Items[i]
		pvName := va.Spec.Source.PersistentVolumeName
		if va.Spec.Attacher != driverName || va.Spec.NodeName != d.k8sNodeName ||
			!va.Status.Attached || pvName == nil {
			continue
		}
		vid, jwt, err := ipACLVolFromPV(ctx, cs, *pvName)
		if err != nil {
			log.WithError(err).WithField("pv", *pvName).Warn("can't load IP-ACL volume from K8s")
			continue
		}
		if vid != nil {
			d.ipACLVols.track(*vid, jwt, nil)
			n++
		}
	}
	return n, nil
}

// ipACLVolFromPV returns the IP-ACL volume of PV `name` and the JWT of its
// node-stage Secret, or nil if the PV is not an IP-ACL volume of this driver.
func ipACLVolFromPV(
	ctx context.Context, cs kubernetes.Interface, name string,
) (*lbResourceID, string, error) {
	pv, err := cs.CoreV1().PersistentVolumes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, "", err
	}
	src := pv.Spec.CSI
	if src == nil || src.Driver != driverName {
		return nil, "", nil
	}
	vid, err := parseCSIResourceID(src.VolumeHandle)
	if err != nil {
		return nil, "", fmt.Errorf("bad volume handle: %s", err)
	}
	if !vid.ipACL {
		return nil, "", nil
	}
	ref := src.NodeStageSecretRef
	if ref == nil {
		return &vid, "", nil
	}
	sec, err := cs.CoreV1().Secrets(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return nil, "", fmt.Errorf("failed to get node-stage Secret %s/%s: %s",
			ref.Namespace, ref.Name, err)
	}
	return &vid, string(sec.Data[jwtSecretKey]), nil
}
