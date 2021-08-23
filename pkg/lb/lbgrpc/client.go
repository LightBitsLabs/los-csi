// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package lbgrpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes"
	guuid "github.com/google/uuid"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_logrus "github.com/grpc-ecosystem/go-grpc-middleware/logging/logrus"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/status"

	"github.com/lightbitslabs/los-csi/pkg/grpcutil"
	"github.com/lightbitslabs/los-csi/pkg/lb"
	mgmt "github.com/lightbitslabs/los-csi/pkg/lb/management"
	"github.com/lightbitslabs/los-csi/pkg/util/endpoint"
	"github.com/lightbitslabs/los-csi/pkg/util/strlist"
	"github.com/lightbitslabs/los-csi/pkg/util/wait"
)

const (
	MaxReplicaCount = 128 // arbitrary, but sensible

	defaultTimeout = 10 * time.Second // ditto

	ifMatchHeader = "if-match" // for ETag
)

var (
	supportedAPIVersions = []string{
		"v2.0",
	}

	CreateRetryOpts = wait.Backoff{
		Delay:      250 * time.Millisecond,
		Factor:     2.0,
		DelayLimit: 2 * time.Second,
		Retries:    8,
	}

	DeleteRetryOpts = wait.Backoff{
		Delay:      200 * time.Millisecond,
		Factor:     2.0,
		DelayLimit: 1 * time.Second,
		Retries:    15,
	}

	UpdateRetryOpts = wait.Backoff{
		Delay:      250 * time.Millisecond,
		Factor:     1.5,
		DelayLimit: 1 * time.Second,
		Retries:    15,
	}

	prng = rand.New(rand.NewSource(time.Now().UnixNano()))
)

// LightOS cluster resolver: -------------------------------------------------

// lbResolver is similar to gRPC manual.Resolver in that it's primed by a number
// of LightOS cluster member addresses. the difference is that lbResolver tries
// to rotate this list of addresses on failures. on the one hand, it keeps the
// client talking mostly to the same mgmt API server avoiding consistency
// issues, but on the other it avoids some of the pathological cases of
// "pick_first" balancer (e.g. the fact that it never even TRIES anything other
// than the "first" if that first was inaccessible on first dial - it just
// burns the entire deadline budget on fruitless attempts to get through to the
// first address and then fails DialContext() on 'i/o timeout'. i guess it's
// just an unfortunate interplay between "pick_first" and default ClientConn
// behaviour - grep for "We can potentially spend all the time trying the
// first address" in addrConn.resetTransport()).
//
// later on this can also be extended quite trivially to update the resolver
// with the latest LightOS cluster member list at runtime based on info fetched
// straight from the horses mouth, to accommodate for added/removed nodes.
//
// oh, yeah, and lbResolver "is also a resolver builder". it's traditional, you
// know...
type lbResolver struct {
	// scheme is not really a URL scheme, but rather a unique per-resolver
	// (or, equivalently, per-LightOS cluster) thing, an artefact of how
	// the dialling gRPC machinery be looking up the resolver later.
	scheme string
	log    *logrus.Entry

	cc resolver.ClientConn

	// mu protects all things EPs related. a bit heavy handed, but trivial
	// and no contention is expected.
	mu   sync.Mutex
	eps  endpoint.Slice // LightOS node EPs in the order to be tried.
	tgts string         // cached string repr of `eps`

}

func newLbResolver(log *logrus.Entry, scheme string, targets endpoint.Slice) *lbResolver {
	log = log.WithField("lb-resolver", scheme)
	r := &lbResolver{
		scheme: scheme,
		eps:    targets.Clone(),
		log:    log,
		tgts:   targets.String(),
	}
	log.WithFields(logrus.Fields{
		"targets": r.tgts,
	}).Info("initialising...")
	return r
}

func (r *lbResolver) Scheme() string {
	return r.scheme
}

// updateCCState() updates the underlying ClientConn with the currently
// known list of LightOS cluster nodes.
func (r *lbResolver) updateCCState() {
	r.mu.Lock()
	addrs := make([]resolver.Address, len(r.eps))
	for i, ep := range r.eps {
		addrs[i].Addr = ep.String()
	}
	r.mu.Unlock()
	r.cc.NewAddress(addrs)
}

// Build() implements (together with Scheme()) the resolver.Builder interface
// and is the way gRPC requests to create a resolver instance when its
// pseudo-scheme is mentioned while dialling. in case of lbResolver, each
// resolver is unique to a LightOS cluster. lbResolver "builds" itself.
func (r *lbResolver) Build(
	target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions,
) (resolver.Resolver, error) {
	r.cc = cc
	r.log.WithFields(logrus.Fields{
		"targets": r.tgts,
	}).Info("building...")
	r.updateCCState()
	return r, nil
}

// ResolveNow() rotates the current list of LightOS cluster node addresses left
// by one and returns it. since this method is typically called when gRPC has
// connectivity problems to the first node in the slice, this achieves the
// effect of making dialer to try to connect to the next node, while putting
// the offending node at the end of the list of nodes to retry.
//
// in the future it could, potentially, be made to trigger querying of the
// LightOS cluster it represents for the latest list of cluster members...
func (r *lbResolver) ResolveNow(o resolver.ResolveNowOptions) {
	r.mu.Lock()
	// not particularly efficient mem-wise, but this should be rare enough
	// event for it not to matter...
	if len(r.eps) > 1 {
		r.eps = append(r.eps[1:], r.eps[0])
	}
	r.tgts = r.eps.String()
	r.mu.Unlock()
	r.log.WithFields(logrus.Fields{
		"targets": r.tgts,
	}).Info("resolving...")
	r.updateCCState()
}

func (r *lbResolver) Close() {
	r.log.Info("closing...")
}

// LightOS gRPC client: ------------------------------------------------------

type Client struct {
	eps  endpoint.Slice
	conn *grpc.ClientConn
	clnt mgmt.DurosAPIClient

	id   string
	tgts string // cached string repr of `eps`
	log  *logrus.Entry

	// peerMu protects all peer-related fields:
	peerMu   sync.Mutex
	lastPeer peer.Peer
	switched bool // a matter of aesthetics: 1st conn shouldn't warn
}

// Dial() creates a LightOS cluster client. it is a blocking call and will only
// return once the connection to [at least one of the] `targets` has been
// actually established - subject to `ctx` limitations. if `ctx` specified
// timeout or duration - dialling (and only dialling!) timeout will be set
// accordingly. `ctx` can also be used to cancel the dialling process, as per
// usual.
//
// the cluster client will make an effort to transparently reconnect to one of
// the `targets` in case of connection loss. if the process of finding a live
// and responsive target amongst `targets` and establishing the connection takes
// longer than the actual operation context timeout (as opposed to the `ctx`
// passed here) - `DeadlineExceeded` will be returned as usual, and the caller
// can retry the operation.
func Dial(ctx context.Context, log *logrus.Entry, targets endpoint.Slice, mgmtScheme string) (*Client, error) {
	if !targets.IsValid() {
		return nil, status.Errorf(codes.InvalidArgument,
			"invalid target endpoints specified: [%s]", targets)
	}
	id := fmt.Sprintf("%07s", strconv.FormatUint(uint64(prng.Uint32()), 36))
	log = log.WithField("clnt-id", id)

	res := &Client{
		eps:  targets.Clone(),
		id:   id,
		tgts: targets.String(),
		log:  log,
	}

	logger := log.WithFields(logrus.Fields{
		"clnt-type": "lbgrpc",
		"targets":   res.tgts,
	})
	logger.Info("connecting...")

	logrusOpts := []grpc_logrus.Option{
		grpc_logrus.WithLevels(grpcutil.LBCodeToLogrusLevel),
	}
	interceptors := []grpc.UnaryClientInterceptor{
		mkUnaryClientInterceptor(res),
		grpc_logrus.UnaryClientInterceptor(log, logrusOpts...),
		grpc_logrus.PayloadUnaryClientInterceptor(log,
			func(context.Context, string) bool { return true },
		),
	}

	// these are broadly in line with the expected server SLOs:
	kal := keepalive.ClientParameters{
		Time:                10 * time.Second,
		Timeout:             10 * time.Second,
		PermitWithoutStream: true,
	}

	// these control only dialling, both initial and subsequent (on server
	// switch-over). it's relatively tight, but then the LightOS clusters
	// controlled by a LB CSI plugin will typically be on the same DC
	// network. given that some COs are fairly aggressive about their call
	// deadlines (for K8s - often 10sec), this should, hopefully, give the
	// client a decent chance to try out at least one more server before
	// the CO call will time out, saving a top-level retry cycle.
	dialBackoffConfig := backoff.Config{
		BaseDelay:  1.0 * time.Second,
		Multiplier: 1.2,
		Jitter:     0.1,
		MaxDelay:   7 * time.Second,
	}
	cp := grpc.ConnectParams{
		Backoff:           dialBackoffConfig,
		MinConnectTimeout: 6 * time.Second,
	}

	scheme := "lightos-" + id
	lbr := newLbResolver(log, scheme, res.eps)

	opts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithDisableRetry(),
		grpc.WithUserAgent("lb-csi-plugin"), // TODO: take from config (?) + add version!
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)),
		grpc.WithUnaryInterceptor(grpc_middleware.ChainUnaryClient(interceptors...)),
		grpc.WithKeepaliveParams(kal),
		grpc.WithConnectParams(cp),
		grpc.WithResolvers(lbr),
	}

	if mgmtScheme == "grpc" {
		logger.Infof("connecting insecurely")
		opts = append(opts, grpc.WithInsecure())
	} else if mgmtScheme == "grpcs" {
		logger.Infof("connecting securely")
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})))
	}

	var err error
	res.conn, err = grpc.DialContext(
		ctx,
		scheme+":///lb-resolver", // use our resolver instead of explicit target
		opts...,
	)
	if err != nil {
		logger.WithField("error", err.Error()).Warn("failed to connect")
		return nil, err
	}

	res.clnt = mgmt.NewDurosAPIClient(res.conn)

	logger.Info("connected!")
	return res, nil
}

// TODO: add stream interceptor *IF* LB API adds streaming entrypoints...
func mkUnaryClientInterceptor(clnt *Client) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, rep interface{}, cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker, opts ...grpc.CallOption,
	) error {
		return clnt.peerReviewUnaryInterceptor(ctx, method, req, rep, cc, invoker, opts...)
	}
}

func (c *Client) peerReviewUnaryInterceptor( // sic!
	ctx context.Context, method string, req, rep interface{}, cc *grpc.ClientConn,
	invoker grpc.UnaryInvoker, opts ...grpc.CallOption,
) error {
	var currPeer peer.Peer
	opts = append(opts, grpc.Peer(&currPeer))
	err := invoker(ctx, method, req, rep, cc, opts...)
	c.peerMu.Lock()

	// TODO: FIXME!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
	if currPeer.Addr != c.lastPeer.Addr {
		// TODO: introduce rate-limiter to spare logs and perf!
		lastPeer := c.lastPeer
		c.lastPeer = currPeer
		c.peerMu.Unlock()
		curr := "<NONE>"
		if currPeer.Addr != nil {
			curr = currPeer.Addr.String()
		}
		last := "<NONE>"
		if lastPeer.Addr != nil {
			last = lastPeer.Addr.String()
		}
		// don't want to warn on healthy flow...
		if c.switched {
			c.log.Warnf("switched target: %s -> %s", last, curr)
		} else {
			c.switched = true
			c.log.Infof("switched target: %s -> %s", last, curr)
		}
	} else {
		c.peerMu.Unlock()
	}
	return err
}

func (c *Client) Close() {
	c.conn.Close()
	c.log.WithField("targets", c.Targets()).Info("disconnected!")
}

func (c *Client) Targets() string {
	return c.tgts
}

func (c *Client) ID() string {
	return c.id
}

func (c *Client) RemoteOk(ctx context.Context) error {
	ver, err := c.clnt.GetVersion(ctx, &mgmt.GetVersionRequest{})
	if err != nil {
		return err
	}

	supportedAPI := false
	for _, v := range supportedAPIVersions {
		if ver.ApiVersion == v {
			supportedAPI = true
			break
		}
	}
	if !supportedAPI {
		return status.Errorf(codes.Unimplemented,
			"LB serves incompatible API version: '%s'", ver.ApiVersion)
	}

	// TODO: check that the cluster is [at least partially] functional and
	// that the responding node is a full-fledged member. unfortunately,
	// currently the API provides no means to ascertain either...
	return nil
}

func (c *Client) GetClusterInfo(ctx context.Context) (*lb.ClusterInfo, error) {
	cluster, err := c.clnt.GetClusterInfo(ctx, &mgmt.GetClusterRequest{})
	if err != nil {
		return nil, err
	}

	uuid, err := guuid.Parse(cluster.UUID)
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"got bad cluster info: bad UUID '%s': %s", cluster.UUID, err)
	}
	if uuid == guuid.Nil {
		return nil, status.Errorf(codes.Internal, "got bad cluster info: UUID is <NIL>")
	}
	if !strings.HasPrefix(cluster.SubsystemNQN, "nqn") {
		return nil, status.Errorf(codes.Internal,
			"got bad cluster info: bad subsystem NQN '%s'", cluster.SubsystemNQN)
	}
	// apparently cluster.supportedMaxReplicas might be 0 during some stages
	// of the cluster boot, but allegedly this is a transient condition...

	return &lb.ClusterInfo{
		UUID:               uuid,
		SubsysNQN:          cluster.SubsystemNQN,
		CurrMaxReplicas:    cluster.CurrentMaxReplicas,
		MaxReplicas:        cluster.SupportedMaxReplicas,
		DiscoveryEndpoints: cluster.DiscoveryEndpoints,
		ApiEndpoints:       cluster.ApiEndpoints,
		NvmeEndpoints:      cluster.NvmeEndpoints,
	}, nil
}

func (c *Client) GetCluster(ctx context.Context) (*lb.Cluster, error) {
	cluster, err := c.clnt.GetCluster(ctx, &mgmt.GetClusterRequest{})
	if err != nil {
		return nil, err
	}

	uuid, err := guuid.Parse(cluster.UUID)
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"got bad cluster info: bad UUID '%s': %s", cluster.UUID, err)
	}
	if uuid == guuid.Nil {
		return nil, status.Errorf(codes.Internal, "got bad cluster info: UUID is <NIL>")
	}
	if !strings.HasPrefix(cluster.SubsystemNQN, "nqn") {
		return nil, status.Errorf(codes.Internal,
			"got bad cluster info: bad subsystem NQN '%s'", cluster.SubsystemNQN)
	}
	// apparently cluster.supportedMaxReplicas might be 0 during some stages
	// of the cluster boot, but allegedly this is a transient condition...

	return &lb.Cluster{
		UUID:               uuid,
		SubsysNQN:          cluster.SubsystemNQN,
		CurrMaxReplicas:    cluster.CurrentMaxReplicas,
		MaxReplicas:        cluster.SupportedMaxReplicas,
		DiscoveryEndpoints: cluster.DiscoveryEndpoints,
		ApiEndpoints:       cluster.ApiEndpoints,
		Capacity:           cluster.Statistics.GetEstimatedFreeLogicalStorage(),
	}, nil
}

func lbNodeStateFromGRPC(c mgmt.DurosNodeInfo_State) lb.NodeState {
	// TODO: a bit of a hack, that... better switch:
	return lb.NodeState(c)
}

func (c *Client) lbNodeFromGRPC(node *mgmt.DurosNodeInfo) (*lb.Node, error) {
	if node == nil {
		return nil, status.Errorf(codes.Internal,
			"got <nil> node from LB with no error")
	}

	if node.Name == "" {
		return nil, status.Errorf(codes.Internal,
			"got bad node from LB: it has invalid empty name")
	}

	uuid, err := guuid.Parse(node.UUID)
	if err != nil || uuid == guuid.Nil {
		return nil, status.Errorf(codes.Internal,
			"got bad node from LB: '%s' has invalid UUID '%s'", node.Name, node.UUID)
	}

	log := c.log.WithFields(logrus.Fields{
		"node-name": node.Name,
		"node-uuid": uuid,
	})

	dataEP, err := endpoint.Parse(node.NvmeEndpoint)
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"got bad node from LB: '%s' has bad data endpoint '%s': %s",
			node.Name, node.NvmeEndpoint, err)
	}

	// TODO: make this a full-blown mandatory check?
	if strings.TrimSpace(node.Hostname) == "" {
		log.Warnf("LB returned node with empty hostname '%s'", node.Hostname)
	}

	switch node.State {
	case mgmt.DurosNodeInfo_Active:
		// the only case where the node is immediately usable
	case mgmt.DurosNodeInfo_Activating,
		mgmt.DurosNodeInfo_Inactive,
		mgmt.DurosNodeInfo_Unattached,
		mgmt.DurosNodeInfo_Attaching,
		mgmt.DurosNodeInfo_Detaching:
		log.Warnf("LB returned node in degraded state '%s' (%d)", node.State, node.State)
	default:
		return nil, status.Errorf(codes.Internal,
			"got bad node from LB: '%s' has unexpected state '%s' (%d)",
			node.Name, node.State, node.State)
	}

	return &lb.Node{
		Name:     node.Name,
		UUID:     uuid,
		DataEP:   dataEP,
		HostName: node.Hostname,
		State:    lbNodeStateFromGRPC(node.State),
	}, nil
}

func (c *Client) ListNodes(ctx context.Context) ([]*lb.Node, error) {
	resp, err := c.clnt.ListNodes(
		ctx,
		&mgmt.ListNodeRequest{},
	)
	if err != nil {
		return nil, err
	}

	nodes := []*lb.Node{}
	errs := []string{}
	for _, node := range resp.Nodes {
		lbNode, err := c.lbNodeFromGRPC(node)
		if err != nil {
			errs = append(errs, err.Error())
		} else {
			nodes = append(nodes, lbNode)
		}
	}

	numGoodNodes := len(nodes)
	numErrs := len(errs)
	if numErrs > 0 {
		return nil, status.Errorf(codes.Unknown,
			"got %d invalid cluster node entries from LB out of %d: [ %s ]",
			numErrs, numGoodNodes+numErrs, strings.Join(errs, "; "))
	}
	if numGoodNodes == 0 {
		return nil, status.Errorf(codes.Unknown,
			"got empty cluster nodes list from LB with no errors")
	}

	return nodes, nil
}

func lbVolumeStateFromGRPC(c mgmt.Volume_StateEnum) lb.VolumeState {
	// TODO: a bit of a hack, that... better switch:
	return lb.VolumeState(c)
}

func lbVolumeProtectionFromGRPC(c mgmt.ProtectionStateEnum) lb.VolumeProtection {
	// TODO: a bit of a hack, that... better switch:
	return lb.VolumeProtection(c)
}

// lbVolumeFromGRPC does basic sanity checks on the volume returned by
// LightOS and converts it to the API-agnostic lb.Volume.
//
// if name, uuid or both are non-nil, the function makes sure the volume
// matches the values pointed to.
func (c *Client) lbVolumeFromGRPC(
	vol *mgmt.Volume, name *string, uuid *guuid.UUID,
) (*lb.Volume, error) {
	if vol == nil {
		return nil, status.Errorf(codes.Internal,
			"got <nil> volume from LB with no error")
	}

	if vol.Name == "" {
		return nil, status.Errorf(codes.Internal,
			"got bad volume from LB: it has invalid empty name")
	}
	if name != nil && vol.Name != *name {
		return nil, status.Errorf(codes.Internal,
			"got wrong volume from LB: '%s' instead of '%s'",
			vol.Name, *name)
	}

	volUUID, err := guuid.Parse(vol.UUID)
	if err != nil || volUUID == guuid.Nil {
		return nil, status.Errorf(codes.Internal,
			"got bad volume from LB: '%s' has invalid UUID '%s'", vol.Name, vol.UUID)
	}
	if uuid != nil && volUUID != *uuid {
		return nil, status.Errorf(codes.Internal,
			"got wrong volume '%s' from LB: UUID %s instead of %s",
			vol.Name, vol.UUID, *uuid)
	}

	switch vol.State {
	case mgmt.Volume_Creating,
		mgmt.Volume_Available,
		mgmt.Volume_Deleting,
		mgmt.Volume_Updating,
		mgmt.Volume_Failed:
		// the only valid states nowadays
	case mgmt.Volume_Deleted:
		// TODO: remove this case once the LightOS API drops the
		// deprecated volume states...
		c.log.WithFields(logrus.Fields{
			"vol-name": vol.Name,
			"vol-uuid": vol.UUID,
		}).Warnf("LB returned volume in deprecated state '%s' (%d)", vol.State, vol.State)
		fallthrough
	default:
		return nil, status.Errorf(codes.Internal,
			"got bad volume from LB: '%s' has unexpected state '%s' (%d)",
			vol.Name, vol.State, vol.State)
	}

	switch vol.ProtectionState {
	case mgmt.ProtectionStateEnum_Unknown:
		// a quirk of implementation: rather than reporting "NotAvailable"
		// while the volume is still in the process of being created, the
		// API returns "Unknown". in all other states it's an indication
		// of a problem...
		if vol.State != mgmt.Volume_Creating {
			return nil, status.Errorf(codes.Internal, "got bad volume from LB: "+
				"'%s' has unexpected protection state '%s' (%d)",
				vol.Name, vol.ProtectionState, vol.ProtectionState)
		}
	case mgmt.ProtectionStateEnum_FullyProtected,
		mgmt.ProtectionStateEnum_Degraded,
		mgmt.ProtectionStateEnum_ReadOnly,
		mgmt.ProtectionStateEnum_NotAvailable:
		// the only currently supported states
	default:
		return nil, status.Errorf(codes.Internal,
			"got bad volume from LB: '%s' has unexpected protection state '%s' (%d)",
			vol.Name, vol.ProtectionState, vol.ProtectionState)
	}

	if vol.ReplicaCount == 0 || vol.ReplicaCount > MaxReplicaCount {
		return nil, status.Errorf(codes.Internal,
			"got bad volume from LB: '%s' has bad replica count of %d",
			vol.Name, vol.ReplicaCount)
	}

	// ...not a word!!!
	var compress bool
	switch strings.ToLower(vol.Compression) {
	case "true":
		compress = true
	case "false":
	default:
		return nil, status.Errorf(codes.Internal,
			"got bad volume from LB: '%s' has bad compression state '%s'",
			vol.Name, vol.Compression)
	}

	return &lb.Volume{
		Name:         vol.Name,
		UUID:         volUUID,
		State:        lbVolumeStateFromGRPC(vol.State),
		Protection:   lbVolumeProtectionFromGRPC(vol.ProtectionState),
		ReplicaCount: vol.ReplicaCount,
		ACL:          strlist.CopyUniqueSorted(vol.Acl.GetValues()),
		Capacity:     vol.Size,
		Compression:  compress,
		ETag:         vol.ETag,
		ProjectName:  vol.ProjectName,
	}, nil
}

func cloneCtxWithCap(ctx context.Context) (context.Context, context.CancelFunc) {
	dl, ok := ctx.Deadline()
	if !ok {
		dl = time.Now().Add(defaultTimeout)
	}
	dl = dl.Add(time.Duration(-10) * time.Millisecond)
	return context.WithDeadline(ctx, dl)
}

func cloneCtxWithETag(ctx context.Context, eTag string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, ifMatchHeader, eTag)
}

func (c *Client) CreateVolume(
	ctx context.Context, name string, capacity uint64, replicaCount uint32,
	compress bool, acl []string, projectName string, snapshotID guuid.UUID, blocking bool, // TODO: refactor options
) (*lb.Volume, error) {
	ctx, cancel := cloneCtxWithCap(ctx)
	defer cancel()

	capStr := strconv.FormatUint(capacity, 10)
	// temporary workaround for server-side parsing issue:
	capStr += "b"

	// sorting/uniqueness are primarily for tracing/debugging/aesthetics,
	// not functional considerations, but ALLOW_NONE is now mandatory:
	acl = strlist.CopyUniqueSorted(acl)
	if len(acl) == 0 {
		acl = []string{lb.ACLAllowNone}
	}

	req := mgmt.CreateVolumeRequest{
		Name:         name,
		Size:         capStr,
		Acl:          &mgmt.StringList{Values: acl},
		Compression:  strconv.FormatBool(compress),
		ReplicaCount: replicaCount,
		ProjectName:  projectName,
	}
	if snapshotID != guuid.Nil {
		req.SourceSnapshotUUID = snapshotID.String()
		c.log.Debugf("creating volume %s from source snapshot uuid: %s",
			req.Name, req.SourceSnapshotUUID)
	}
	vol, err := c.clnt.CreateVolume(ctx, &req)
	if err != nil {
		return nil, err
	}

	lbVol, err := c.lbVolumeFromGRPC(vol, &name, nil)
	if err != nil {
		return nil, err
	}
	uuid := lbVol.UUID

	log := c.log.WithFields(logrus.Fields{
		"vol-name": lbVol.Name,
		"vol-uuid": lbVol.UUID,
	})

	if !strlist.AreEqual(lbVol.ACL, acl) {
		// both ACLs were normalised, so these are no order-related discrepancies:
		return nil, status.Errorf(codes.Internal,
			"LB created volume '%s' with bad ACL %#q", name, lbVol.ACL)
	}

	warnOnRO := func() {
		// currently we don't support target-side-RO volumes, so this is
		// unexpected. on the other hand - let the caller handle it.
		if !lbVol.IsWritable() {
			log.Warnf("LB created volume that is not currently usable: "+
				"its protection state is '%s'", lbVol.Protection)
		}
	}

	// strictly speaking, LB CreateVolume() should only ever return volumes
	// in 'Creating' or 'Available' states, but once burned - twice shy...
	switch lbVol.State {
	case lb.VolumeDeleting,
		lb.VolumeUpdating,
		lb.VolumeFailed:
		log.Warnf("LB volume creation returned volume in unexpected state '%s' (%d)",
			lbVol.State, lbVol.State)
		return nil, status.Errorf(codes.Internal,
			"LB created volume '%s' in inappropriate state '%s' (%d)",
			name, lbVol.State, lbVol.State)
	case lb.VolumeCreating:
		if blocking {
			break
		}
		fallthrough
	case lb.VolumeAvailable:
		warnOnRO()
		return lbVol, nil
	default:
		// this can only happen if new state handling was added to
		// lbVolumeFromGRPC() but not to this switch, which is an
		// out and out bug here, so distinctive msg:
		return nil, status.Errorf(codes.Internal, "LB created volume '%s' in unexpected "+
			"state '%s' (%d) that's not handled by the client yet",
			name, lbVol.State, lbVol.State)
	}

	// upon caller's request, wait for the volume to be fully created:
	orgLbVol := lbVol
	err = wait.WithExponentialBackoff(CreateRetryOpts, func() (bool, error) {
		if err := ctx.Err(); err != nil {
			return false, grpcutil.ErrFromCtxErr(err)
		}

		lbVol, err = c.GetVolume(ctx, uuid, projectName)
		if err != nil {
			return false, err
		}

		diffs := orgLbVol.ExplainDiffsFrom(lbVol, "created", "obtained", false)
		if len(diffs) > 0 {
			// this might have been a race with some other instance,
			// or, more likely at this stage, an API issue...
			return false, status.Errorf(codes.Internal,
				"volume '%s' properties changed while waiting for it to be "+
					"created: %s", vol.Name, strings.Join(diffs, ", "))
		}

		if !strlist.AreEqual(orgLbVol.ACL, lbVol.ACL) {
			// this is likely just a race with some other instance...
			log.Warnf("got volume with unexpected ACL %#q instead of %#q while "+
				"waiting for volume to be created", lbVol.ACL, orgLbVol.ACL)
		}

		switch lbVol.State {
		case lb.VolumeCreating:
			// play it again, Sam...
			return false, nil
		case lb.VolumeDeleting:
			// apparently a race with a DeleteVolume() invoked by CO.
			// `Canceled` would have made more sense, but the closest
			// the CSI spec gets is `Aborted`...
			return false, status.Errorf(codes.Aborted,
				"volume appears to have been deleted in parallel")
		case lb.VolumeFailed:
			log.Warnf("got bad volume from LB after create: '%s' is in state %s (%d)",
				name, lbVol.State, lbVol.State)
			// LB failed to create a volume (e.g. too many nodes are
			// down). try to cause the CO to retry the whole shebang
			// anew.
			return false, status.Errorf(codes.Unavailable,
				"LB failed to create volume '%s', try again later", name)
		case lb.VolumeUpdating:
			// another apparent race with a different call invoked
			// by a CO, this time with UpdateVolume(). since the latter
			// is only supposed to be accepted for volumes in
			// `Available` state, we can deduce that this volume is
			// effectively `Available` as well.
			fallthrough
		case lb.VolumeAvailable:
			return true, nil
		default:
			return false, status.Errorf(codes.Internal,
				"volume '%s' entered unexpected state while waiting for it "+
					"to be created: %s (%d)", name, lbVol.State, lbVol.State)
		}
	})
	if err != nil {
		return nil, err
	}

	warnOnRO()
	return lbVol, nil
}

func isStatusNotFound(err error) bool {
	st, ok := status.FromError(err)
	if ok && st.Code() == codes.NotFound {
		return true
	}
	return false
}

func (c *Client) DeleteVolume(
	ctx context.Context, uuid guuid.UUID, projectName string, blocking bool, // TODO: refactor options
) error {
	ctx, cancel := cloneCtxWithCap(ctx)
	defer cancel()

	_, err := c.clnt.DeleteVolume(
		ctx,
		&mgmt.DeleteVolumeRequest{
			UUID:        uuid.String(),
			ProjectName: projectName,
		},
	)
	if err != nil || !blocking {
		return err
	}

	// upon caller's request, wait for the volume deletion to kick off
	// (though NOT necessarily complete!) so that another identically named
	// volume can be created immediately upon return from this function:
	err = wait.WithExponentialBackoff(DeleteRetryOpts, func() (bool, error) {
		if err := ctx.Err(); err != nil {
			return false, grpcutil.ErrFromCtxErr(err)
		}

		lbVol, err := c.GetVolume(ctx, uuid, projectName)
		if err != nil {
			if isStatusNotFound(err) {
				return true, nil
			}
			return false, err
		}

		log := c.log.WithFields(logrus.Fields{
			"vol-name": lbVol.Name,
			"vol-uuid": lbVol.UUID,
		})

		switch lbVol.State {
		case lb.VolumeAvailable:
			// play it again, Sam...
			return false, nil
		case lb.VolumeCreating,
			lb.VolumeUpdating:
			// this is somewhat unlikely, as the original DeleteVolume()
			// API call should have failed...
			log.Warnf("got volume in unexpected state from LB after delete: %s (%d)",
				lbVol.State, lbVol.State)
			// moreover, this volume is unlikely to transition from
			// 'Creating' to 'Deleted' on its own. let admins or K8s
			// sort this out (the latter is likely to retry deleting).
			return false, status.Errorf(codes.Unavailable,
				"LB failed to delete volume '%s', try again later", lbVol.Name)
		case lb.VolumeFailed:
			// also highly unlikely, if it was already 'Failed', the
			// original DeleteVolume() would have failed too, and a
			// volume can't just hop into 'Failed' state afterwards.
			log.Warnf("got volume in unexpected state from LB after delete: %s (%d)",
				lbVol.State, lbVol.State)
			// still, it's gone, so...
			fallthrough
		case lb.VolumeDeleting:
			return true, nil
		default:
			return false, status.Errorf(codes.Internal,
				"volume '%s' entered unexpected state while waiting for it to be "+
					"deleted: %s (%d)", lbVol.Name, lbVol.State, lbVol.State)
		}
	})
	return err
}

func (c *Client) getVolume(
	ctx context.Context, name *string, uuid *guuid.UUID, projectName *string,
) (*lb.Volume, error) {
	ctx, cancel := cloneCtxWithCap(ctx)
	defer cancel()

	req := mgmt.GetVolumeRequest{}
	if name != nil {
		req.Name = *name
	}
	if projectName != nil {
		req.ProjectName = *projectName
	}
	if uuid != nil {
		req.UUID = uuid.String()
	}
	vol, err := c.clnt.GetVolume(ctx, &req)
	if err != nil {
		return nil, err
	}

	return c.lbVolumeFromGRPC(vol, name, uuid)
}

func (c *Client) GetVolume(ctx context.Context, uuid guuid.UUID, projectName string) (*lb.Volume, error) {
	return c.getVolume(ctx, nil, &uuid, &projectName)
}

func (c *Client) GetVolumeByName(ctx context.Context, name string, projectName string) (*lb.Volume, error) {
	return c.getVolume(ctx, &name, nil, &projectName)
}

// doUpdateVolume() implements a single cycle of GetVolume() -> patch ->
// UpdateVolume() on behalf of the callers that are expected to do this in a
// loop, normally using wait.WithExponentialBackoff(). hence if it returns
// neither error nor volume - that's the hint for the caller to retry. the
// errors that it returns otherwise are gRPC status errors suitable for
// direct passthrough to the callers.
func (c *Client) doUpdateVolume(
	ctx context.Context, uuid guuid.UUID, projectName string, hook lb.VolumeUpdateHook,
) (*lb.Volume, error) {
	ctx, cancel := cloneCtxWithCap(ctx)
	defer cancel()

	log := c.log.WithField("vol-uuid", uuid)

	badError := func(op, prep string, err error) (*lb.Volume, error) {
		log.WithFields(logrus.Fields{
			"err-type": fmt.Sprintf("%T", err),
			"err-msg":  err.Error(),
		}).Errorf("unexpected error on volume update: failed to %s volume", op)
		return nil, status.Errorf(codes.Unknown,
			"failed to %s volume %s %s LB: %s", op, uuid, prep, err)
	}

	lbVol, err := c.GetVolume(ctx, uuid, projectName)
	if err != nil {
		st, ok := status.FromError(err)
		if !ok {
			return badError("get", "from", err)
		}
		code := st.Code()
		switch code {
		case codes.NotFound:
			return nil, err
		case codes.Unavailable:
			// retry locally or return DeadlineExceeded if ctx
			// expired for the CO to [presumably] retry.
			return nil, nil
		default:
			return badError("get", "from", err)
		}
	}
	log = log.WithField("vol-name", lbVol.Name)

	switch lbVol.State {
	case lb.VolumeCreating:
		log.Warn("trying to update volume that's still being created")
		fallthrough
	case lb.VolumeUpdating:
		// retry locally or return DeadlineExceeded if ctx expired for
		// the CO to [presumably] retry.
		return nil, nil
	case lb.VolumeDeleting,
		lb.VolumeFailed:
		// apparently either a race with a DeleteVolume() invoked by
		// a CO, or UpdateVolume() was invoked without checking that
		// the volume was successfully created to begin with. since
		// this particular volume is not coming back in either case -
		// just pretend it doesn't exist.
		return nil, status.Errorf(codes.NotFound, "no such volume")
	case lb.VolumeAvailable:
		// the only one that can be updated currently.
	default:
		// this can only happen if new state handling was added to
		// lbVolumeFromGRPC() but not to this switch, which is an
		// out and out bug here, so distinctive msg:
		return nil, status.Errorf(codes.Internal, "can't update volume in unexpected "+
			"state %s (%d) that's not handled by the client yet",
			lbVol.State, lbVol.State)
	}

	update, err := hook(lbVol)
	if err != nil {
		log.Debugf("volume update aborted by hook: %s", err)
		// propagate the hook error to the caller verbatim so they can
		// recognise it, if necessary:
		return nil, err
	}
	if update == nil {
		// presumably the hook recognised that the volume reached its
		// desired state. all done!
		log.Debug("no further volume update requested by hook")
		return lbVol, nil
	}

	if err := ctx.Err(); err != nil {
		return nil, grpcutil.ErrFromCtxErr(err)
	}
	ctx = cloneCtxWithETag(ctx, lbVol.ETag)

	req := mgmt.UpdateVolumeRequest{
		UUID:        uuid.String(),
		ProjectName: projectName,
	}
	required := false
	if update.ACL != nil {
		required = true
		acl := strlist.CopyUniqueSorted(update.ACL)
		req.Acl = &mgmt.StringList{Values: acl}
		log = log.WithField("acl-src", fmt.Sprintf("%#q", lbVol.ACL))
		log = log.WithField("acl-tgt", fmt.Sprintf("%#q", acl))
	}
	if update.Capacity != 0 {
		required = true
		req.Size = fmt.Sprintf("%d", update.Capacity)
		log = log.WithField("capacity-src", fmt.Sprintf("%d", lbVol.Capacity))
		log = log.WithField("capacity-tgt", fmt.Sprintf("%d", update.Capacity))
	}
	if !required {
		// bug, code not updated to match some newly added lb.VolumeUpdate
		// field, or the caller goofed and passed an empty request.
		log.Warn("volume update requested by hook, but no updates recognised")
		return lbVol, nil
	}
	log = log.WithField("etag", lbVol.ETag)
	log.Debug("volume update requested by hook")

	// currently UpdateVolume() response is empty...
	_, err = c.clnt.UpdateVolume(
		ctx,
		&req,
	)
	if err != nil {
		st, ok := status.FromError(err)
		if !ok {
			return badError("update", "on", err)
		}
		code := st.Code()
		switch code {
		case codes.NotFound:
			return nil, err
		case codes.Unavailable:
			// retry locally or return DeadlineExceeded if ctx
			// expired for the CO to [presumably] retry.
			return nil, nil
		case codes.InvalidArgument:
			log.Errorf("volume update refused by LB on bad arg: %s", st.Message())
			return nil, status.Errorf(codes.Internal,
				"failed to update volume %s on LB: %s", uuid, st.Message())
		case codes.FailedPrecondition:
			// this one's a bit of a pickle: originally this meant
			// ETag mismatch, but at some point the server started
			// to return this on updates to volumes in unexpected
			// states (at least 'Deleting'/'Failed') too. since it's
			// impossible to figure out the reason merely from the
			// UpdateVolume() response - retry the whole thing and
			// hope the logic above will weed out the bad states so
			// we don't end up in an infinite loop:
			log.Debugf("will retry volume update refused by LB on failed "+
				"precondition: %s", st.Message())
		default:
			return badError("update", "on", err)
		}
	}

	// update request was accepted by the LB. keep retrying locally until
	// it's actually carried out:
	return nil, nil
}

// UpdateVolume() attempts to *overwrite* the fields of the volume specified
// by `uuid` with non-nil/default fields of `update` return value of the `hook`,
// see VolumeUpdateHook docs for details. it handles verifying that the update
// successfully completed and handles some of the common errors (such as
// retrying on codes.Unavailable, concurrent racing updates, etc.) internally,
// but delegates the higher-level application logic to the `hook`.
func (c *Client) UpdateVolume(
	ctx context.Context, uuid guuid.UUID, projectName string, hook lb.VolumeUpdateHook,
) (*lb.Volume, error) {
	var lbVol *lb.Volume
	err := wait.WithExponentialBackoff(UpdateRetryOpts, func() (bool, error) {
		var err error
		if err = ctx.Err(); err != nil {
			return false, grpcutil.ErrFromCtxErr(err)
		}

		lbVol, err = c.doUpdateVolume(ctx, uuid, projectName, hook)
		return err == nil && lbVol != nil, err
	})
	if err != nil {
		// NOTE: may include hook-specific errors that are NOT grpc.Status!
		return nil, err
	}
	return lbVol, nil
}

func (c *Client) getSnapshot(
	ctx context.Context, name *string, uuid *guuid.UUID, projectName string,
) (*lb.Snapshot, error) {
	ctx, cancel := cloneCtxWithCap(ctx)
	defer cancel()

	req := mgmt.GetSnapshotRequest{
		ProjectName: projectName,
	}
	if name != nil {
		req.Name = *name
	}
	if uuid != nil {
		req.UUID = uuid.String()
	}

	// TODO add projectName for multi-tenancy support
	snap, err := c.clnt.GetSnapshot(ctx, &req)
	if err != nil {
		return nil, err
	}

	return c.lbSnapshotFromGRPC(snap, name, uuid)
}

func (c *Client) GetSnapshot(ctx context.Context, uuid guuid.UUID, projectName string) (*lb.Snapshot, error) {
	return c.getSnapshot(ctx, nil, &uuid, projectName)
}

func (c *Client) GetSnapshotByName(ctx context.Context, name string, projectName string) (*lb.Snapshot, error) {
	return c.getSnapshot(ctx, &name, nil, projectName)
}

func (c *Client) CreateSnapshot(
	ctx context.Context, name string, projectName string,
	srcVolUUID guuid.UUID, blocking bool,
) (*lb.Snapshot, error) {
	ctx, cancel := cloneCtxWithCap(ctx)
	defer cancel()

	snap, err := c.clnt.CreateSnapshot(
		ctx,
		&mgmt.CreateSnapshotRequest{
			Name:             name,
			SourceVolumeUUID: srcVolUUID.String(),
			Description:      "K8S Snapshot: Volume UUID: " + srcVolUUID.String(),
			ProjectName:      projectName,
		},
	)
	if err != nil {
		st, ok := status.FromError(err)
		if !ok {
			return nil, err
		}
		code := st.Code()
		switch code {
		case codes.InvalidArgument:
			c.log.Errorf("create snapshot refused by LB on bad arg: %s", st.Message())
			return nil, status.Errorf(codes.Internal,
				"failed to create snapshot %s on LB: %s", name, st.Message())
		case codes.FailedPrecondition:
			// most likely source volume is being updated, tell
			// upper layers to retry the whole thing and
			// hope the logic above will weed out the bad states so
			// we don't end up in an infinite loop:
			c.log.Debugf("create snapshot refused by LB on failed "+
				"precondition: %s", st.Message())
			return nil, status.Errorf(codes.Unavailable,
				"create snapshot (%s) transiently failed", name)
		}
		return nil, err
	}

	lbSnap, err := c.lbSnapshotFromGRPC(snap, &name, nil)
	if err != nil {
		return nil, err
	}
	uuid := lbSnap.UUID

	log := c.log.WithFields(logrus.Fields{
		"snap-name": lbSnap.Name,
		"snap-uuid": lbSnap.UUID,
	})

	switch lbSnap.State {
	case lb.SnapshotDeleting,
		lb.SnapshotFailed:
		log.Warnf("LB snapshot creation returned volume in unexpected state '%s' (%d)",
			lbSnap.State, lbSnap.State)
		return nil, status.Errorf(codes.Internal,
			"LB created snapshot '%s' in inappropriate state '%s' (%d)",
			name, lbSnap.State, lbSnap.State)
	case lb.SnapshotCreating:
		if blocking {
			break
		}
		fallthrough
	case lb.SnapshotAvailable:
		return lbSnap, nil
	default:
		return nil, status.Errorf(codes.Internal, "LB created snapshot '%s' in unexpected "+
			"state '%s' (%d) that's not handled by the client yet",
			name, lbSnap.State, lbSnap.State)
	}

	// upon caller's request, wait for the snapshot to be fully created:
	err = wait.WithExponentialBackoff(CreateRetryOpts, func() (bool, error) {
		if err := ctx.Err(); err != nil {
			return false, grpcutil.ErrFromCtxErr(err)
		}

		lbSnap, err = c.GetSnapshot(ctx, uuid, projectName)
		if err != nil {
			return false, err
		}

		if srcVolUUID != lbSnap.SrcVolUUID {
			return false, status.Errorf(codes.Internal,
				"LB snapshot source volume UUID mismatch (srcVolUUID=%s, snapshot.srcVolUUID=%s",
				srcVolUUID, lbSnap.SrcVolUUID)
		}

		switch lbSnap.State {
		case lb.SnapshotCreating:
			// play it again, Sam...
			return false, nil
		case lb.SnapshotDeleting:
			// FIXME: if we return ReadyToUse == false, this race is impossible?
			return false, status.Errorf(codes.Aborted,
				"snapshot appears to have been deleted in parallel")
		case lb.SnapshotFailed:
			log.Warnf("got bad snapshot from LB after create: '%s' is in state %s (%d)",
				name, lbSnap.State, lbSnap.State)
			return false, status.Errorf(codes.Unavailable,
				"LB failed to create volume '%s', try again later", name)
		case lb.SnapshotAvailable:
			return true, nil
		default:
			return false, status.Errorf(codes.Internal,
				"snapshot '%s' entered unexpected state while waiting for it "+
					"to be created: %s (%d)", name, lbSnap.State, lbSnap.State)
		}
	})
	if err != nil {
		return nil, err
	}

	return lbSnap, nil
}

func (c *Client) DeleteSnapshot(
	ctx context.Context, uuid guuid.UUID, projectName string, blocking bool,
) error {
	ctx, cancel := cloneCtxWithCap(ctx)
	defer cancel()

	_, err := c.clnt.DeleteSnapshot(
		ctx,
		&mgmt.DeleteSnapshotRequest{
			UUID:        uuid.String(),
			ProjectName: projectName,
		},
	)
	if err != nil || !blocking {
		st, ok := status.FromError(err)
		if !ok {
			return err
		}
		code := st.Code()
		switch code {
		case codes.InvalidArgument:
			c.log.Errorf("delete snapshot refused by LB on bad arg: %s", st.Message())
			return status.Errorf(codes.Internal,
				"failed to delete snapshot %s on LB: %s", uuid, st.Message())
		case codes.FailedPrecondition:
			// most likely source volume is being updated, tell
			// upper layers to retry the whole thing and
			// hope the logic above will weed out the bad states so
			// we don't end up in an infinite loop:
			c.log.Debugf("delete snapshot refused by LB on failed "+
				"precondition: %s", st.Message())
			return status.Errorf(codes.Unavailable,
				"delete snapshot (%s) transiently failed", uuid.String())
		}
		return err
	}

	err = wait.WithExponentialBackoff(DeleteRetryOpts, func() (bool, error) {
		if err := ctx.Err(); err != nil {
			return false, grpcutil.ErrFromCtxErr(err)
		}

		lbSnap, err := c.GetSnapshot(ctx, uuid, projectName)
		if err != nil {
			if isStatusNotFound(err) {
				return true, nil
			}
			return false, err
		}

		log := c.log.WithFields(logrus.Fields{
			"snap-name": lbSnap.Name,
			"snap-uuid": lbSnap.UUID,
		})

		switch lbSnap.State {
		case lb.SnapshotAvailable:
			return false, nil
		case lb.SnapshotCreating:
			log.Warnf("got snapshot in unexpected state from LB after delete: %s (%d)",
				lbSnap.State, lbSnap.State)
			return false, status.Errorf(codes.Unavailable,
				"LB failed to delete snapshot '%s', try again later", lbSnap.Name)
		case lb.SnapshotFailed:
			log.Warnf("got snapshot in unexpected state from LB after delete: %s (%d)",
				lbSnap.State, lbSnap.State)
			fallthrough
		case lb.SnapshotDeleting:
			return true, nil
		default:
			return false, status.Errorf(codes.Internal,
				"snapshot '%s' entered unexpected state while waiting for it to be "+
					"deleted: %s (%d)", lbSnap.Name, lbSnap.State, lbSnap.State)
		}
	})
	return err
}

func lbSnapshotStateFromGRPC(c mgmt.Snapshot_StateEnum) lb.SnapshotState {
	return lb.SnapshotState(c)
}

// lbSnapshotFromGRPC converted from lbVolumeFromGRPC.
func (c *Client) lbSnapshotFromGRPC(
	snap *mgmt.Snapshot, name *string, uuid *guuid.UUID,
) (*lb.Snapshot, error) {
	if snap == nil {
		return nil, status.Errorf(codes.Internal,
			"got <nil> snap from LB with no error")
	}
	if snap.Name == "" {
		return nil, status.Errorf(codes.Internal,
			"got bad snapshot from LB: it has invalid empty name")
	}
	if name != nil && snap.Name != *name {
		return nil, status.Errorf(codes.Internal,
			"got wrong snap from LB: '%s' instead of '%s'",
			snap.Name, *name)
	}

	snapUUID, err := guuid.Parse(snap.UUID)
	if err != nil || snapUUID == guuid.Nil {
		return nil, status.Errorf(codes.Internal,
			"got bad snapshot from LB: '%s' has invalid UUID '%s'", snap.Name, snap.UUID)
	}
	if uuid != nil && snapUUID != *uuid {
		return nil, status.Errorf(codes.Internal,
			"got wrong snapshot '%s' from LB: UUID %s instead of %s",
			snap.Name, snap.UUID, *uuid)
	}
	SrcVolUUID, err := guuid.Parse(snap.SourceVolumeUUID)
	if err != nil || SrcVolUUID == guuid.Nil {
		return nil, status.Errorf(codes.Internal,
			"got bad snapshot from LB: '%s' has invalid SrcVolUUID '%s'",
			snap.Name, snap.SourceVolumeUUID)
	}

	switch snap.State {
	case mgmt.Snapshot_Creating,
		mgmt.Snapshot_Available,
		mgmt.Snapshot_Deleting,
		mgmt.Snapshot_Failed:
	default:
		return nil, status.Errorf(codes.Internal,
			"got bad snapshot from LB: '%s' has unexpected state '%s' (%d)",
			snap.Name, snap.State, snap.State)
	}

	if snap.CreationTime == nil {
		snap.CreationTime = ptypes.TimestampNow()
	}

	return &lb.Snapshot{
		Name:               snap.Name,
		UUID:               snapUUID,
		Capacity:           snap.Size,
		SrcVolUUID:         SrcVolUUID,
		SrcVolName:         snap.SourceVolumeName,
		SrcVolReplicaCount: snap.ReplicaCount,
		SrcVolCompression:  snap.Compression,
		CreationTime:       snap.CreationTime,
		State:              lbSnapshotStateFromGRPC(snap.State),
		ETag:               snap.ETag,
		ProjectName:        snap.ProjectName,
	}, nil
}
