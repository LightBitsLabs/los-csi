// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"fmt"
	"net"

	"github.com/sirupsen/logrus"

	"github.com/lightbitslabs/los-csi/pkg/lb"
	"github.com/lightbitslabs/los-csi/pkg/util/endpoint"
	"github.com/lightbitslabs/los-csi/pkg/util/strlist"
)

// maxIPAclEntries mirrors the per-volume IP-ACL entry cap enforced by the
// LightOS API service.
const maxIPAclEntries = 16

// dataPathAddrs returns the unique local addresses the kernel will use as
// the source of connections towards `eps`, derived by route lookup. no
// packets are sent in the process.
func dataPathAddrs(eps endpoint.Slice) ([]string, error) {
	addrs := []string{}
	for _, ep := range eps {
		conn, err := net.Dial("udp", ep.String())
		if err != nil {
			continue
		}
		if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
			addrs = append(addrs, addr.IP.String())
		}
		_ = conn.Close()
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no route to any of the endpoints '%s'", eps)
	}
	return strlist.CopyUniqueSorted(addrs), nil
}

// enrollNodeIPAcl adds this node's data-path source addresses to the volume
// IP-ACL, so that the subsequent NVMe/TCP connections from this node are
// admitted by the target. it is idempotent, and it leaves an ALLOW_ANY set
// externally on the volume untouched.
func (d *Driver) enrollNodeIPAcl(
	ctx context.Context, log *logrus.Entry, clnt lb.Client, ci *lb.ClusterInfo, vid lbResourceID,
) error {
	eps, err := endpoint.ParseSliceIP(ci.DiscoveryEndpoints)
	if err != nil {
		return mkEExec("got unusable discovery endpoints from LB cluster at '%s': %s",
			vid.mgmtEPs[0], err)
	}
	addrs, err := dataPathAddrs(eps)
	if err != nil {
		return mkEExec("failed to derive node data-path addresses: %s", err)
	}
	log = log.WithField("ip-acl-addrs", fmt.Sprintf("%#q", addrs))

	hook := func(vol *lb.Volume) (*lb.VolumeUpdate, error) {
		if strlist.Contains(vol.IPAcl, lb.ACLAllowAny) {
			return nil, nil
		}
		ipACL := strlist.Remove(strlist.CopyUniqueSorted(vol.IPAcl), lb.ACLAllowNone)
		missing := false
		for _, addr := range addrs {
			if !strlist.Contains(ipACL, addr) {
				ipACL = append(ipACL, addr)
				missing = true
			}
		}
		if !missing {
			return nil, nil
		}
		if len(ipACL) > maxIPAclEntries {
			return nil, mkPrecond("volume IP-ACL can't accommodate node "+
				"addresses %#q: %d entries, limit is %d",
				addrs, len(ipACL), maxIPAclEntries)
		}
		return &lb.VolumeUpdate{IPAcl: ipACL}, nil
	}

	vol, err := clnt.UpdateVolume(ctx, vid.uuid, vid.projName, hook)
	if err != nil {
		return err
	}
	if strlist.Contains(vol.IPAcl, lb.ACLAllowAny) {
		return nil
	}
	for _, addr := range addrs {
		if !strlist.Contains(vol.IPAcl, addr) {
			// either some race involving network partitions, or, an
			// external intervention. a retry will either sort it out,
			// or report the condition more accurately:
			log.WithFields(logrus.Fields{
				"ip-acl-exp": fmt.Sprintf("%#q", addrs),
				"ip-acl-got": fmt.Sprintf("%#q", vol.IPAcl),
			}).Error("UpdateVolume() succeeded, but resultant volume IP-ACL is wrong")
			return mkEagain("failed to enroll node addresses in IP-ACL of volume '%s'",
				vid.uuid)
		}
	}
	return nil
}
