// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/rest"
)

const k8sListAttempts = 5

// trackIPAclVolsFromK8s refills the IP-ACL re-check list at startup. kubelet
// does not stage the volumes again after a plugin restart, so the list is
// rebuilt from the volumes attached to this node, with the JWT of their
// node-stage Secret.
func (d *Driver) trackIPAclVolsFromK8s(ctx context.Context) {
	log := d.log.WithFields(logrus.Fields{"op": "IPAclRecheck", "node-id": d.nodeID})

	att, err := newK8sNodeAttachments()
	if errors.Is(err, rest.ErrNotInCluster) {
		log.Debug("not in a K8s cluster, IP-ACL re-check list not loaded")
		return
	}
	if err != nil {
		log.WithError(err).Warn("can't create K8s client")
		return
	}

	for attempt := 1; ; attempt++ {
		n, err := d.loadIPAclVolsFromK8s(ctx, log, att)
		if err == nil {
			log.WithField("volumes", n).Info("IP-ACL re-check list loaded from K8s")
			return
		}
		log.WithError(err).WithFields(logrus.Fields{"volumes": n, "attempt": attempt}).
			Warn("failed to load IP-ACL re-check list from K8s")
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
	ctx context.Context, log *logrus.Entry, att *k8sNodeAttachments,
) (int, error) {
	vols, err := att.attachedVolumes(ctx, d.nodeID)
	if err != nil {
		return 0, err
	}
	n, failed := 0, 0
	for _, v := range vols {
		if !v.attached {
			continue
		}
		vid, err := parseCSIResourceID(v.id)
		if err != nil {
			log.WithError(err).WithField("volume-id", v.id).Warn("unparsable volume ID")
			continue
		}
		if !vid.ipACL {
			continue
		}
		jwt, err := att.stageJWT(ctx, v.stageSecret)
		if err != nil {
			log.WithError(err).WithField("vol-uuid", vid.uuid).Warn("can't load IP-ACL volume from K8s")
			failed++
			continue
		}
		d.ipACLVols.track(vid, jwt, nil)
		n++
	}
	if failed > 0 {
		return n, fmt.Errorf("failed to load %d of the volumes attached to the node", failed)
	}
	return n, nil
}
