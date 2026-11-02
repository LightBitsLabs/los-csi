// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"fmt"
	"sync"
	"time"

	guuid "github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/lightbitslabs/los-csi/pkg/lb"
	"github.com/lightbitslabs/los-csi/pkg/util/strlist"
)

const (
	// ipACLRecheckPeriod must stay well below the time the discovery
	// client keeps reconnecting a lost NVMe/TCP controller.
	ipACLRecheckPeriod  = 30 * time.Second
	ipACLRecheckTimeout = 10 * time.Second

	jwtSecretKey = "jwt"
)

// ipACLVol is a staged volume whose IP-ACL the node keeps enrolled. entries
// are replaced, never modified, so a pointer identifies one tracking period.
type ipACLVol struct {
	vid   lbResourceID
	jwt   string   // empty: use the global JWT, like the stage does.
	addrs []string // enrolled last; nil until the first enrollment.
}

// ipACLVolList holds the IP-ACL volumes staged on this node.
type ipACLVolList struct {
	mtx  sync.Mutex
	vols map[guuid.UUID]*ipACLVol
}

// track adds or refreshes a volume. nil `addrs` keeps the addresses already
// known, so a stage retry does not cause a needless IP-ACL update.
func (l *ipACLVolList) track(vid lbResourceID, jwt string, addrs []string) {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	if l.vols == nil {
		l.vols = map[guuid.UUID]*ipACLVol{}
	}
	if cur, ok := l.vols[vid.uuid]; ok && addrs == nil {
		addrs = cur.addrs
	}
	l.vols[vid.uuid] = &ipACLVol{vid: vid, jwt: jwt, addrs: addrs}
}

func (l *ipACLVolList) untrack(uuid guuid.UUID) {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	delete(l.vols, uuid)
}

func (l *ipACLVolList) list() []*ipACLVol {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	res := make([]*ipACLVol, 0, len(l.vols))
	for _, v := range l.vols {
		res = append(res, v)
	}
	return res
}

// setAddrs and drop act only if `v` is still the tracked entry: a stage or
// unstage during a re-check wins over its result.
func (l *ipACLVolList) setAddrs(v *ipACLVol, addrs []string) {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	if l.vols[v.vid.uuid] == v {
		l.vols[v.vid.uuid] = &ipACLVol{vid: v.vid, jwt: v.jwt, addrs: addrs}
	}
}

func (l *ipACLVolList) drop(v *ipACLVol) {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	if l.vols[v.vid.uuid] == v {
		delete(l.vols, v.vid.uuid)
	}
}

func jwtCreds(jwt string) map[string]string {
	if jwt == "" {
		return nil
	}
	return map[string]string{jwtSecretKey: jwt}
}

func (d *Driver) recheckIPAclLoop(ctx context.Context) {
	ticker := time.NewTicker(ipACLRecheckPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.recheckIPAcl(ctx)
		}
	}
}

// recheckIPAcl enrolls the node's current data-path source addresses in the
// IP-ACL of every staged IP-ACL volume. the addresses can change after the
// stage: the node's data IP changes, or a target added later is reached
// through another interface.
func (d *Driver) recheckIPAcl(ctx context.Context) {
	byCluster := map[string][]*ipACLVol{}
	for _, v := range d.ipACLVols.list() {
		key := fmt.Sprintf("%s|%s", v.vid.scheme, v.vid.mgmtEPs)
		byCluster[key] = append(byCluster[key], v)
	}
	for _, vols := range byCluster {
		d.recheckClusterIPAcl(ctx, vols)
	}
}

func (d *Driver) recheckClusterIPAcl(ctx context.Context, vols []*ipACLVol) {
	vid := vols[0].vid
	log := d.log.WithFields(logrus.Fields{
		"op":      "IPAclRecheck",
		"mgmt-ep": vid.mgmtEPs,
	})

	cctx := d.cloneCtxWithCreds(ctx, jwtCreds(vols[0].jwt))
	clnt, err := d.GetLBClient(cctx, vid.mgmtEPs, vid.scheme)
	if err != nil {
		log.WithError(err).Warn("can't connect to LB cluster")
		return
	}
	defer d.PutLBClient(clnt)

	cictx, cancel := context.WithTimeout(cctx, ipACLRecheckTimeout)
	ci, err := clnt.GetClusterInfo(cictx)
	cancel()
	if err != nil {
		log.WithError(err).Warn("failed to get LB cluster info")
		return
	}
	addrs, err := nodeDataPathAddrs(ci, vid)
	if err != nil {
		log.WithError(err).Warn("failed to derive node data-path addresses")
		return
	}

	for _, v := range vols {
		if !strlist.AreEqual(v.addrs, addrs) {
			d.recheckVolIPAcl(ctx, log, clnt, v, addrs)
		}
	}
}

func (d *Driver) recheckVolIPAcl(
	ctx context.Context, log *logrus.Entry, clnt lb.Client, v *ipACLVol, addrs []string,
) {
	log = log.WithFields(logrus.Fields{"vol-uuid": v.vid.uuid, "project": v.vid.projName})
	vctx, cancel := context.WithTimeout(d.cloneCtxWithCreds(ctx, jwtCreds(v.jwt)), ipACLRecheckTimeout)
	defer cancel()

	reset, err := updateNodeIPAcl(vctx, log, clnt, v.vid, addrs, true)
	switch {
	case status.Code(err) == codes.NotFound:
		d.ipACLVols.drop(v)
		log.Info("volume gone, IP-ACL re-check stopped")
	case err != nil:
		log.WithError(err).Warn("failed to update volume IP-ACL")
	case reset:
		d.ipACLVols.drop(v)
		log.Info("volume IP-ACL reset, IP-ACL re-check stopped")
	default:
		d.ipACLVols.setAddrs(v, addrs)
		log.WithField("ip-acl-addrs", fmt.Sprintf("%#q", addrs)).
			Info("node data-path addresses enrolled in volume IP-ACL")
	}
}
