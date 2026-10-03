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

// nodeDataPathAddrs derives the node source addresses towards the cluster
// portals, data portals included: on nodes with several data-path interfaces
// each portal may route out a different one, and every source the kernel
// will use must be admitted.
func nodeDataPathAddrs(ci *lb.ClusterInfo, vid lbResourceID) ([]string, error) {
	eps, err := endpoint.ParseSliceIP(
		append(append([]string{}, ci.DiscoveryEndpoints...), ci.NvmeEndpoints...))
	if err != nil {
		return nil, mkEExec("got unusable target endpoints from LB cluster at '%s': %s",
			vid.mgmtEPs[0], err)
	}
	addrs, err := dataPathAddrs(eps)
	if err != nil {
		return nil, mkEExec("failed to derive node data-path addresses: %s", err)
	}
	return addrs, nil
}

// mergeNodeIPAcl returns the volume IP-ACL with `addrs` enrolled, or nil if
// no update is needed. an ALLOW_ANY set externally is left untouched, an
// ALLOW_NONE placeholder is displaced by the first real entry.
func mergeNodeIPAcl(cur, addrs []string) ([]string, error) {
	if strlist.Contains(cur, lb.ACLAllowAny) {
		return nil, nil
	}
	have := strlist.Remove(strlist.CopyUniqueSorted(cur), lb.ACLAllowNone)
	ipACL := strlist.CopyUniqueSorted(append(have, addrs...))
	if len(ipACL) == len(have) {
		return nil, nil
	}
	if len(ipACL) > maxIPAclEntries {
		return nil, mkPrecond("volume IP-ACL can't accommodate node addresses "+
			"%#q: %d entries, limit is %d. fully detach the volume to reset "+
			"its IP-ACL", addrs, len(ipACL), maxIPAclEntries)
	}
	return ipACL, nil
}

// enrollNodeIPAcl adds this node's data-path source addresses to the volume
// IP-ACL, so that the subsequent NVMe/TCP connections from this node are
// admitted by the target. it is idempotent, and it leaves an ALLOW_ANY set
// externally on the volume untouched.
func (d *Driver) enrollNodeIPAcl(
	ctx context.Context, log *logrus.Entry, clnt lb.Client, ci *lb.ClusterInfo, vid lbResourceID,
) error {
	addrs, err := nodeDataPathAddrs(ci, vid)
	if err != nil {
		return err
	}
	log = log.WithField("ip-acl-addrs", fmt.Sprintf("%#q", addrs))

	hook := func(vol *lb.Volume) (*lb.VolumeUpdate, error) {
		ipACL, err := mergeNodeIPAcl(vol.IPAcl, addrs)
		if err != nil {
			log.WithField("ip-acl-got", fmt.Sprintf("%#q", vol.IPAcl)).
				Error("can't enroll node addresses in volume IP-ACL")
			return nil, err
		}
		if ipACL == nil {
			return nil, nil
		}
		return &lb.VolumeUpdate{IPAcl: ipACL}, nil
	}

	_, err = clnt.UpdateVolume(ctx, vid.uuid, vid.projName, hook)
	return err
}
