// Copyright (C) 2016--2021 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"context"

	guuid "github.com/google/uuid"
	"google.golang.org/grpc/status"

	"github.com/lightbitslabs/los-csi/pkg/lb"
	"github.com/lightbitslabs/los-csi/pkg/util/endpoint"
)

// targetEnv describes the LightOS cluster environment that will be providing
// the underlying storage for a given CSI volume in terms of NVMe/NVMe-oF
// protocol level details. this information should be sufficient for a compliant
// Backend impl to cause the underlying NVMe-oF host to connect to the
// appropriate NVMe-oF targets exposed by the LightOS cluster and present a
// block device on the local node for the rest of the CSI plugin to work with.
type TargetEnv struct {
	SubsysNQN    string
	DiscoveryEPs endpoint.Slice
	NvmeEPs      endpoint.Slice
}

// the LB CSI plugin delegates the handling of the local Linux block devices
// representing the namespaces exported by the remote LightOS targets to the
// Backend instances. examples of such Backend-s might be the local Linux kernel
// NVMe-oF host (initiator) implementation, various SmartNICs offloading the
// NVMe-oF functionality and presenting the namespaces as local NVMe devices to
// the client Linux kernel, etc.
//
// Backend implementations are expected to take care of everything necessary to
// surface a remote namespace locally as a block device upon volume attachment
// to the node and removal of said block device upon detachment. this will
// typically include connecting/disconnecting to/from the targets as necessary,
// handling the NVMe ANA/multipathing and failover, etc.
//
// moreover, Backend implementations are expected to make do with interacting
// with LightOS targets at the NVMe-oF level only, without resorting to LightOS
// management API calls. this deliberately limits the scope of Backend
// responsibilities and allows for Backends that are thin wrappers around
// NVMe-oF host implementations.
//
// the LB CSI plugin guarantees that there will be only one outstanding call per
// Attach()/Detach() method for a given `nguid` within this instance of the LB
// CSI plugin at a time, however multiple calls for different NGUID-s might be
// in-flight simultaneously. backends are expected to handle such concurrency
// gracefully.
//
// Backend methods are expected to return gRPC-compatible Status objects
// encapsulating the error on failures or nil on success. the resultant errors
// are propagated back through the CSI methods VERBATIM, so the method
// implementers are advised to think carefully about the desired resulting
// Container Orchestrator behaviour.
//
// informative log entries upon entry and exit into/from the methods will be
// highly appreciated. a pseudo-backend `backend.Wrapper` can be used to
// generate those automatically for actual Backend implementations.
type Backend interface {
	// Type SHALL return a unique human-readable (and preferably short!)
	// string identifying this backend implementation.
	Type() string

	// LBVolEligible() SHALL return nil if the backend assesses that it will
	// be able to successfully attach the volume described by `vol` to this
	// CO host, currently - based on the information contained in `vol`
	// (properties of the volume itself), in the future - possibly based on
	// additional criteria as well. otherwise LBVolEligible() SHALL return
	// an error describing why attaching the volume will be impossible.
	// see also Attach() below.
	//
	// LBVolEligible() allows to rule out impossible scenarios early on. it
	// is called by Driver.lbVolEligible() as part of the `NodeStageVolume`
	// flow, assuming the other, backend-agnostic eligibility conditions
	// were met. e.g. if the backend doesn't support replicated/striped/etc.
	// volumes the backend can abort the attachment process.
	LBVolEligible(ctx context.Context, vol *lb.Volume) *status.Status

	// Attach() SHALL result in a block device '/dev/nvmeXnY' corresponding
	// to the remote namespace with NGUID `nguid` being present on the local
	// node.
	//
	// Attach will only be invoked if a Backend reported no error on a
	// corresponding preceding call to LBVolEligible().
	Attach(ctx context.Context, tgtEnv *TargetEnv, nguid guuid.UUID) *status.Status

	// Detach() SHOULD result in a block device '/dev/nvmeXnY' corresponding
	// to the remote namespace with the NGUID `nguid` disappearing from the
	// local node, preferably immediately, or, failing that, eventually.
	//
	// this clause is phrased in such an awkward way because the Linux
	// kernel NVMe-oF host driver impl does not provide a race-free way to
	// disconnect from a controller if the target might export multiple NSes
	// on the same controller. one userspace process might decide that the
	// last NS from a given controller should be Detach()-ed (so the host
	// should disconnect from the controller altogether), while in parallel,
	// another one might decide that a different NS from the same controller
	// should be Attach()-ed. if the 2nd process wins the race, the loser
	// process might inadvertently cause the NVMe-oF host to disconnect from
	// the controller, potentially losing data already submitted for writes
	// to the 2nd NS, and certainly effectively Detach()-ing the 2nd NS -
	// which should not have been Detach()-ed.
	Detach(ctx context.Context, nguid guuid.UUID) *status.Status
}
