// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

//nolint:gosec
package lb_test

import (
	"context"
	"fmt"
	"math/rand"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	guuid "github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/lightbitslabs/los-csi/pkg/lb"
	"github.com/lightbitslabs/los-csi/pkg/util/endpoint"
)

const (
	defaultTimeout = 3 * time.Second
	leeway         = 10 * time.Millisecond
)

// would it have killed the core team to just export the lockedSource?
type lockedSource struct {
	mu  sync.Mutex
	src rand.Source64
}

func (r *lockedSource) Int63() int64 {
	r.mu.Lock()
	n := r.src.Int63()
	r.mu.Unlock()
	return n
}

func (r *lockedSource) Uint64() uint64 {
	r.mu.Lock()
	n := r.src.Uint64()
	r.mu.Unlock()
	return n
}

func (r *lockedSource) Seed(seed int64) {
	r.mu.Lock()
	r.src.Seed(seed)
	r.mu.Unlock()
}

var (
	lockedPrngSource = lockedSource{
		src: rand.NewSource(time.Now().UnixNano()).(rand.Source64),
	}
	prng *rand.Rand
)

func init() {
	rand.Seed(time.Now().UnixNano())
	lockedPrngSource.Seed(time.Now().UnixNano())
	prng = rand.New(&lockedPrngSource)
}

type zipfRange struct {
	min  uint64
	max  uint64
	zipf *rand.Zipf
}

// newZipfRange creates new Zipf-based variate generator. `min` and `max`
// are assumed to have been validated.
func newZipfRange(min, max uint64) *zipfRange {
	v := float64(max-min) / 5
	if v < 1.0 {
		v = 1.0
	}
	return &zipfRange{
		min: min,
		max: max,
		zipf: rand.NewZipf(
			prng,
			// hand-tuned params to give around 80% in bottom 10th
			// percentile and long but rapidly tapering tail:
			5.0,
			v,
			max-min,
		),
	}
}

func (z *zipfRange) rand() uint64 { //revive:disable-line:import-shadowing // huh?
	return z.min + z.zipf.Uint64()
}

// FakeClient: ---------------------------------------------------------------

type fakeClientOptions struct {
	minDialDelayUsec uint64 // inclusive
	maxDialDelayUsec uint64 // inclusive
	minOpDelayUsec   uint64 // inclusive
	maxOpDelayUsec   uint64 // inclusive
}

type fakeClient struct {
	id          string
	env         *testEnv
	targets     endpoint.Slice
	closedCount int64
	opts        fakeClientOptions
	opZipf      *zipfRange
}

const (
	blockForeverPort = 1000
	failPromptlyPort = 2000
)

//revive:disable:unused-parameter,unused-receiver

// fakeClientDial() will fake dialling, as per `opts` - except as specified
// below. `targets[0]` must be well-formed (though not necessarily real) for
// dialling to "succeed". dialling to the following ports will always fail,
// regardless of `opts`:
//     1000 - will block forever or past `ctx` deadline
//     2000 - will fail immediately
func fakeClientDial(
	ctx context.Context, env *testEnv, targets endpoint.Slice, mgmtScheme string, //nolint:unparam
	opts fakeClientOptions,
) (*fakeClient, error) {
	if opts.maxDialDelayUsec < opts.minDialDelayUsec {
		panic(fmt.Sprintf("FakeClientDial: maxDialDelayUsec (%d) < minDialDelayUsec (%d)",
			opts.maxDialDelayUsec, opts.minDialDelayUsec))
	}
	if opts.maxOpDelayUsec < opts.minOpDelayUsec {
		panic(fmt.Sprintf("FakeClientDial: maxOpDelayUsec (%d) < minOpDelayUsec (%d)",
			opts.maxOpDelayUsec, opts.minOpDelayUsec))
	}

	if !targets.IsValid() {
		return nil, status.Errorf(codes.InvalidArgument,
			"invalid target endpoints specified: [%s]", targets)
	}

	dialZipf := newZipfRange(opts.minDialDelayUsec, opts.maxDialDelayUsec)
	dialDelay := time.Duration(dialZipf.rand()) * time.Microsecond

	port := targets[0].Port()
	if port == failPromptlyPort {
		return nil, fmt.Errorf("transport: error while dialing: dial tcp %s: "+
			"connect: connection refused", targets[0])
	}
	if port == blockForeverPort {
		dialDelay = 24 * 365 * time.Hour
	}

	select {
	case <-ctx.Done():
		err := ctx.Err()
		switch err {
		case context.Canceled:
			return nil, status.Error(codes.Canceled, "context cancelled")
		case context.DeadlineExceeded:
			return nil, status.Errorf(codes.DeadlineExceeded,
				"timed out while connecting to LB")
		default:
			return nil, status.Errorf(codes.Unknown,
				"call to LB aborted for unexpected reason: %s", err)
		}
	case <-time.After(dialDelay):
	}

	atomic.AddInt64(&env.numClients, 1)
	atomic.AddInt64(&env.totalClients, 1)

	return &fakeClient{
		id:      fmt.Sprintf("%07s", strconv.FormatUint(uint64(prng.Uint32()), 36)),
		env:     env,
		targets: targets,
		opts:    opts,
		opZipf:  newZipfRange(opts.minOpDelayUsec, opts.maxOpDelayUsec),
	}, nil
}

func (c *fakeClient) Close() {
	if !atomic.CompareAndSwapInt64(&c.closedCount, 0, 1) {
		n := atomic.AddInt64(&c.closedCount, 1)
		c.env.t.Errorf("BUG: client.Close(%s) called %d times for targets '%s'",
			c.id, n, c.targets)
	}

	atomic.AddInt64(&c.env.numClients, -1)
}

func (c *fakeClient) Targets() string {
	return c.targets.String()
}

func (c *fakeClient) ID() string {
	return c.id
}

func (c *fakeClient) RemoteOk(ctx context.Context) error {
	if atomic.LoadInt64(&c.closedCount) > 0 {
		return status.Errorf(codes.Canceled,
			"grpc: the client connection is closing")
	}
	time.Sleep(time.Duration(c.opZipf.rand()) * time.Microsecond)
	return nil
}

func (c *fakeClient) GetCluster(ctx context.Context) (*lb.Cluster, error) {
	return nil, nil
}

func (c *fakeClient) GetClusterInfo(ctx context.Context) (*lb.ClusterInfo, error) {
	return nil, nil
}

func (c *fakeClient) ListNodes(ctx context.Context) ([]*lb.Node, error) {
	return nil, nil
}

func (c *fakeClient) CreateVolume(
	ctx context.Context, name string, capacity uint64,
	replicaCount uint32, compress bool, acl []string,
	projectName string, snapshotID guuid.UUID,
	qosPolicyName string, blocking bool, // TODO: refactor options
) (*lb.Volume, error) {
	return nil, nil
}

func (c *fakeClient) DeleteVolume(
	ctx context.Context, uuid guuid.UUID, projectName string, blocking bool,
) error {
	return nil
}

func (c *fakeClient) GetVolume(
	ctx context.Context, uuid guuid.UUID, projectName string,
) (*lb.Volume, error) {
	return nil, nil
}

func (c *fakeClient) GetVolumeByName(
	ctx context.Context, name string, projectName string,
) (*lb.Volume, error) {
	return nil, nil
}

func (c *fakeClient) UpdateVolume(
	ctx context.Context, uuid guuid.UUID, projectName string, hook lb.VolumeUpdateHook,
) (*lb.Volume, error) {
	return nil, nil
}

func (c *fakeClient) CreateSnapshot(
	ctx context.Context, name string, projectName string, srcVolUUID guuid.UUID,
	descr string, blocking bool,
) (*lb.Snapshot, error) {
	return nil, nil
}

func (c *fakeClient) DeleteSnapshot(
	ctx context.Context, uuid guuid.UUID, projectName string, blocking bool,
) error {
	return nil
}

func (c *fakeClient) GetSnapshot(
	ctx context.Context, uuid guuid.UUID, projectName string,
) (*lb.Snapshot, error) {
	return nil, nil
}

func (c *fakeClient) GetSnapshotByName(
	ctx context.Context, name string, projectName string,
) (*lb.Snapshot, error) {
	return nil, nil
}

//revive:enable:unused-parameter,unused-receiver

// Test env: -----------------------------------------------------------------

type testEnv struct {
	t            *testing.T
	pool         *lb.ClientPool
	numClients   int64
	totalClients int64
}

func (env *testEnv) assertNumClients(expected int64) {
	actual := atomic.LoadInt64(&env.numClients)
	if actual < 0 {
		env.t.Fatalf("BUG: SNAFU: negative number of clients: %d", actual)
	} else if actual != expected {
		env.t.Fatalf("BUG: expected %d live clients, found %d", expected, actual)
	}
}

func (env *testEnv) assertNoClients() {
	n := atomic.LoadInt64(&env.numClients)
	if n < 0 {
		env.t.Fatalf("BUG: SNAFU: negative number of clients: %d", n)
	} else if n > 0 {
		env.t.Fatalf("BUG: %d clients weren't closed and leaked!", n)
	}
}

func (env *testEnv) assertTotalClients(expected int64) {
	actual := atomic.LoadInt64(&env.totalClients)
	if actual != expected {
		env.t.Fatalf("BUG: expected %d clients to have existed, but "+
			"%d were created", expected, actual)
	}
}

func (env *testEnv) assertGetClient(
	ctx context.Context, targets endpoint.Slice, format string, args ...interface{},
) lb.Client {
	clnt, err := env.pool.GetClient(ctx, targets, "grpc")
	if err != nil {
		env.t.Fatalf("BUG: GetClient(%s) failed: "+format,
			append([]interface{}{targets}, args...)...)
	}
	return clnt
}

func (env *testEnv) assertGetNoClient(
	ctx context.Context, targets endpoint.Slice, format string,
	args ...interface{}, //nolint:unparam
) {
	clnt, err := env.pool.GetClient(ctx, targets, "grpc")
	if err == nil {
		env.t.Fatalf("BUG: GetClient(%s) unexpectedly succeeded with client ID %s: "+
			format, append([]interface{}{targets, clnt.ID()}, args...)...)
	}
	if clnt != nil {
		env.t.Fatalf("BUG: GetClient(%s) returned non-nil client ID %s on error '%s'"+
			format, append([]interface{}{targets, clnt.ID()}, args...)...)
	}
}

func (env *testEnv) assertUseClientOk(clnt lb.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	err := clnt.RemoteOk(ctx)
	if err != nil {
		env.t.Fatalf("BUG: client.RemoteOk() failed: '%s'", err)
	}
}

func (env *testEnv) assertNotBefore(deadline time.Time, format string, args ...interface{}) {
	if time.Now().Before(deadline) {
		env.t.Fatalf("BUG: "+format, args...)
	}
}

func (env *testEnv) assertNotAfter(deadline time.Time, format string, args ...interface{}) {
	if time.Now().After(deadline.Add(leeway)) {
		env.t.Errorf("BUG: "+format, args...)
	}
}

// checkGoroutineLeaks checks if the test leaked any goroutines (duh!).
// there are a few more or less usable leaktest packages out there, but for
// a simple test such an external dependency is overkill... yes, the
// output will include a few extra Go runtime goroutines, but you're
// not supposed to stare at it too often, so...
func checkGoroutineLeaks(t *testing.T) func() {
	startgr := runtime.NumGoroutine()

	return func() {
		// ugly hack. without it this check occasionally catches one
		// of Go runtime's goroutines running, throwing off the count.
		// if a test leaked one out - it'll still be there afterwards.
		time.Sleep(20 * time.Millisecond)
		endgr := runtime.NumGoroutine()
		if endgr > startgr {
			buf := make([]byte, 4*1024*1024)
			n := runtime.Stack(buf, true)
			buf = buf[:n]
			t.Fatalf("BUG: %d goroutines may have been leaked. "+
				"all stacks:\n%s", endgr-startgr, buf)
		}
	}
}

// Tests: --------------------------------------------------------------------

var simplePoolOpts = lb.ClientPoolOptions{
	DialTimeout: 3 * time.Second,
	LingerTime:  10 * time.Second,
}

func runSmoke(t *testing.T, st smokeTest) {
	defer checkGoroutineLeaks(t)()

	env := testEnv{
		t: t,
	}

	defer env.assertNoClients()
	poolOpts := simplePoolOpts
	if st.poolOpts != nil {
		poolOpts = *st.poolOpts
	}
	env.pool = lb.NewClientPoolWithOptions(
		func(ctx context.Context, targets endpoint.Slice, mgmtScheme string) (lb.Client, error) {
			return fakeClientDial(ctx, &env, targets, mgmtScheme, st.clientOpts)
		},
		poolOpts,
	)
	defer env.pool.Close()
	st.f(&env)
}

func TestSmoke(t *testing.T) {
	for _, tc := range smokeTests {
		tc := tc // capture range
		t.Run(tc.name, func(t *testing.T) {
			runSmoke(t, tc)
		})
	}
}

type smokeTest struct {
	name       string
	f          func(env *testEnv)
	poolOpts   *lb.ClientPoolOptions
	clientOpts fakeClientOptions
}

var quickDecayCPO = &lb.ClientPoolOptions{
	LingerTime: 1 * time.Millisecond,
	ReapCycle:  500 * time.Microsecond,
}

var smokeTests = []smokeTest{
	{name: "CreateDeletePool", f: testCreateDeletePool},
	{name: "GetUsePut", f: testGetUsePut},
	{name: "ClosePoolInUse", f: testClosePoolInUse},
	{name: "UseClientAfterPoolClose", f: testUseClientAfterPoolClose},
	{name: "DialTimeout", f: testDialTimeout},
	{name: "DialFailPromptly", f: testDialFailPromptly},
	{name: "GetTimeout", f: testGetTimeout, poolOpts: &lb.ClientPoolOptions{
		DialTimeout: 10 * time.Second,
	}},
	{name: "GetCancel", f: testGetCancel, poolOpts: &lb.ClientPoolOptions{
		DialTimeout: 10 * time.Second,
	}},
	{name: "GetGetDiffClusters", f: testGetGetDiffClusters},
	{name: "GetGetSameCluster", f: testGetGetSameCluster},
	{name: "GetGetSameClusterABAB", f: testGetGetSameClusterABAB, poolOpts: quickDecayCPO},
	{name: "GetGetDiffClustersABBA", f: testGetGetDiffClustersABBA, poolOpts: quickDecayCPO},
	{name: "GetGetDiffClustersABAC", f: testGetGetDiffClustersABAC, poolOpts: quickDecayCPO},
	{name: "GetExpireGet", f: testGetExpireGet, poolOpts: quickDecayCPO},
	{name: "2ndMouse", f: test2ndMouse, poolOpts: nil, clientOpts: fakeClientOptions{
		minDialDelayUsec: 200 * 1000,
		maxDialDelayUsec: 200 * 1000,
	}},
}

func mkCtx(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

func testCreateDeletePool(_ *testEnv) {
	// merely to invoke this test case, runSmoke() will have to create a
	// pool and then tear it down once this TC is done, so it's not a NOP.
	time.Sleep(50 * time.Millisecond)
}

func mkTargets(targets string) endpoint.Slice {
	return endpoint.MustParseCSV(targets)
}

var (
	hostA     = mkTargets("1.0.0.1:80")
	hostB     = mkTargets("2.0.0.2:80")
	hostC     = mkTargets("3.0.0.3:80")
	hostBlock = mkTargets(fmt.Sprintf("1.0.0.1:%d", blockForeverPort))
	hostFail  = mkTargets(fmt.Sprintf("1.0.0.1:%d", failPromptlyPort))
)

func concat(l, r endpoint.Slice) endpoint.Slice {
	x := append(l[:0:0], l...) //nolint:gocritic // gocritic bug: that's how you clone a slice!
	return append(x, r...)
}

func testGetUsePut(env *testEnv) {
	ctx, _ := mkCtx(10 * time.Second)
	clnt := env.assertGetClient(ctx, hostA, "bummer...", "grpc")
	env.assertNumClients(1)
	env.assertUseClientOk(clnt)
	env.pool.PutClient(clnt)
	env.assertTotalClients(1)
}

func testClosePoolInUse(env *testEnv) {
	ctx, _ := mkCtx(10 * time.Second)
	clnt := env.assertGetClient(ctx, hostA, "bummer...", "grpc")
	env.assertNumClients(1)
	env.assertUseClientOk(clnt)
	env.assertTotalClients(1)
}

func testUseClientAfterPoolClose(env *testEnv) {
	ctx, _ := mkCtx(10 * time.Second)
	clnt := env.assertGetClient(ctx, hostA, "bummer...", "grpc")
	env.assertNumClients(1)
	env.assertUseClientOk(clnt)
	env.pool.Close()
	env.assertNumClients(0)

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	err := clnt.RemoteOk(ctx)
	if err == nil {
		env.t.Fatalf("BUG: RemoteOk() succeeded past pool.Close()")
	}
	env.assertTotalClients(1)
}

func testDialTimeout(env *testEnv) {
	ctx, _ := mkCtx(250 * time.Millisecond)
	env.assertGetNoClient(ctx, hostBlock, "should have timed out")
	env.assertNumClients(0)
	env.assertTotalClients(0)
}

func testDialFailPromptly(env *testEnv) {
	ctx, _ := mkCtx(250 * time.Millisecond)
	env.assertGetNoClient(ctx, hostFail, "dial should have promptly failed")
	env.assertNumClients(0)
	env.assertTotalClients(0)
}

func testGetTimeout(env *testEnv) {
	deadline := time.Now().Add(100 * time.Microsecond)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	env.assertGetNoClient(ctx, hostBlock, "should have timed out")
	env.assertNumClients(0)
	env.assertNotBefore(deadline, "GetClient(%s) returned before ctx deadline", hostBlock)
	env.assertNotAfter(deadline, "GetClient(%s) ignored ctx deadline", hostBlock)
	env.assertTotalClients(0)
}

func testGetCancel(env *testEnv) {
	ctx, cancel := mkCtx(20 * time.Second)
	cancelDelay := 100 * time.Millisecond
	deadline := time.Now().Add(cancelDelay)
	go func() {
		time.Sleep(cancelDelay)
		cancel()
	}()
	env.assertGetNoClient(ctx, hostBlock, "should have gotten cancelled")
	env.assertNumClients(0)
	env.assertNotBefore(deadline, "GetClient(%s) returned before ctx cancel", hostBlock)
	env.assertNotAfter(deadline, "GetClient(%s) ignored ctx canellation", hostBlock)
	env.assertTotalClients(0)
}

func testGetGetDiffClusters(env *testEnv) {
	ctx, _ := mkCtx(10 * time.Second)
	clnt1 := env.assertGetClient(ctx, hostA, "#1")
	clnt2 := env.assertGetClient(ctx, hostB, "#2")
	if clnt1 == clnt2 {
		env.t.Fatalf("BUG: GetClient(%s) returned the same client as GetClient(%s)",
			hostA, hostB)
	}
	env.assertNumClients(2)
	env.pool.PutClient(clnt1)
	env.pool.PutClient(clnt2)
	env.assertNumClients(2)
	env.assertTotalClients(2)
}

func testGetGetSameCluster(env *testEnv) {
	ctx, _ := mkCtx(10 * time.Second)
	clnt1 := env.assertGetClient(ctx, hostA, "#1")
	clnt2 := env.assertGetClient(ctx, hostA, "#2")
	if clnt1 != clnt2 {
		env.t.Fatalf("BUG: GetClient(%s) returned different clients", hostA)
	}
	env.assertNumClients(1)
	env.pool.PutClient(clnt1)
	env.assertNumClients(1)
	env.assertTotalClients(1)
}

func testGetGetSameClusterABAB(env *testEnv) {
	ctx, _ := mkCtx(10 * time.Second)
	tgtAB := concat(hostA, hostB)
	clnt1 := env.assertGetClient(ctx, tgtAB, "#1")
	clnt2 := env.assertGetClient(ctx, tgtAB, "#2")
	if clnt1 != clnt2 {
		env.t.Fatalf("BUG: GetClient(%s) returned diff clients", tgtAB)
	}
	env.assertNumClients(1)
	env.pool.PutClient(clnt1)
	time.Sleep(10 * time.Millisecond)
	env.assertNumClients(1)
	env.pool.PutClient(clnt2)
	time.Sleep(10 * time.Millisecond)
	env.assertNumClients(0)
	env.assertTotalClients(1)
}

func testGetGetDiffClustersABBA(env *testEnv) {
	ctx, _ := mkCtx(10 * time.Second)
	tgtAB := concat(hostA, hostB)
	tgtBA := concat(hostB, hostA)
	clnt1 := env.assertGetClient(ctx, tgtAB, "AB")
	clnt2 := env.assertGetClient(ctx, tgtBA, "BA")
	if clnt1 == clnt2 {
		env.t.Fatalf("BUG: GetClient() on '%s' & '%s' returned same client", tgtAB, tgtBA)
	}
	env.assertNumClients(2)
	env.pool.PutClient(clnt1)
	time.Sleep(10 * time.Millisecond)
	env.assertNumClients(1)
	env.pool.PutClient(clnt2)
	time.Sleep(10 * time.Millisecond)
	env.assertNumClients(0)
	env.assertTotalClients(2)
}

func testGetGetDiffClustersABAC(env *testEnv) {
	ctx, _ := mkCtx(10 * time.Second)
	tgtAB := concat(hostA, hostB)
	tgtAC := concat(hostA, hostC)
	clnt1 := env.assertGetClient(ctx, tgtAB, "AB")
	clnt2 := env.assertGetClient(ctx, tgtAC, "AC")
	if clnt1 == clnt2 {
		env.t.Fatalf("BUG: GetClient() on '%s' & '%s' returned same client", tgtAB, tgtAC)
	}
	env.assertNumClients(2)
	env.pool.PutClient(clnt1)
	time.Sleep(10 * time.Millisecond)
	env.assertNumClients(1)
	env.pool.PutClient(clnt2)
	time.Sleep(10 * time.Millisecond)
	env.assertNumClients(0)
	env.assertTotalClients(2)
}

func testGetExpireGet(env *testEnv) {
	ctx, _ := mkCtx(10 * time.Second)
	clnt1 := env.assertGetClient(ctx, hostA, "#1")
	env.assertNumClients(1)
	env.pool.PutClient(clnt1)
	time.Sleep(10 * time.Millisecond)
	env.assertNumClients(0)
	clnt2 := env.assertGetClient(ctx, hostA, "#2")
	env.assertNumClients(1)
	env.pool.PutClient(clnt2)
	env.assertNumClients(1)
	if clnt1 == clnt2 {
		env.t.Fatalf("BUG: GetClient(%s) returned same client despite expire", hostA)
	}
	env.assertTotalClients(2)
}

func test2ndMouse(env *testEnv) {
	deadline := time.Now().Add(100 * time.Millisecond)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	env.assertGetNoClient(ctx, hostA, "should have timed out")
	env.assertNotBefore(deadline, "GetClient(%s) returned before ctx deadline", hostA)
	env.assertNotAfter(deadline, "GetClient(%s) ignored ctx deadline", hostA)
	env.assertNumClients(0)

	ctx, _ = mkCtx(1 * time.Second)
	clnt2 := env.assertGetClient(ctx, hostA, "#2")
	env.assertNumClients(1)
	env.pool.PutClient(clnt2)
	env.assertNumClients(1)
	env.assertTotalClients(1)
}

func fuzzOne(
	ctx context.Context, env *testEnv, targets endpoint.Slice, failed chan<- struct{},
) {
	switch targets[0].Port() {
	case failPromptlyPort:
		deadline := time.Now().Add(1 * time.Millisecond)
		tmpCtx, cancel := context.WithDeadline(ctx, deadline)
		defer cancel()
		_, err := env.pool.GetClient(tmpCtx, targets, "grpc")
		if err == nil {
			env.t.Errorf("BUG: GetClient(%s) unexpectedly succeeded", targets)
			failed <- struct{}{}
			return
		}
		if time.Now().After(deadline.Add(leeway * 100)) {
			env.t.Errorf("BUG: GetClient(%s) ignored ctx deadline", targets)
			failed <- struct{}{}
			return
		}
	case blockForeverPort:
		deadline := time.Now().Add(100 * time.Millisecond)
		tmpCtx, cancel := context.WithDeadline(ctx, deadline)
		defer cancel()
		_, err := env.pool.GetClient(tmpCtx, targets, "grpc")
		if err == nil {
			env.t.Errorf("BUG: GetClient(%s) unexpectedly succeeded", targets)
			failed <- struct{}{}
			return
		}
		if time.Now().After(deadline.Add(leeway * 100)) {
			env.t.Errorf("BUG: GetClient(%s) ignored ctx deadline", targets)
			failed <- struct{}{}
			return
		}
	default:
		tmpCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		clnt, err := env.pool.GetClient(ctx, targets, "grpc")
		if err != nil {
			if tmpCtx.Err() == nil {
				env.t.Errorf("BUG: GetClient(%s) failed: %s", targets, err)
				failed <- struct{}{}
			}
			return
		}
		defer env.pool.PutClient(clnt)
		ops := rand.Int63n(3)
		for i := int64(0); i < ops; i++ {
			err := clnt.RemoteOk(ctx)
			if err != nil {
				if tmpCtx.Err() == nil {
					env.t.Errorf("BUG: GetClient(%s) failed: %s", targets, err)
					failed <- struct{}{}
				}
				return
			}
		}
	}
}

func fuzzUser(
	ctx context.Context, env *testEnv, clusters []endpoint.Slice, failed chan<- struct{},
) {
	numClusters := int64(len(clusters))
	for {
		select {
		case <-ctx.Done():
			return
		default:
			targets := clusters[rand.Int63n(numClusters)]
			fuzzOne(ctx, env, targets, failed)
		}
	}
}

func TestFuzz(t *testing.T) {
	defer checkGoroutineLeaks(t)()
	env := testEnv{
		t: t,
	}
	defer env.assertNoClients()

	testTime := 9 * time.Minute
	if testing.Short() {
		testTime = 10 * time.Second
	}

	poolOpts := lb.ClientPoolOptions{
		DialTimeout: 550 * time.Millisecond,
		LingerTime:  500 * time.Microsecond,
		ReapCycle:   100 * time.Microsecond,
	}
	clientOpts := fakeClientOptions{
		minDialDelayUsec: 100,
		maxDialDelayUsec: 500000, // 500 msec
		minOpDelayUsec:   50,
		maxOpDelayUsec:   5000, // 5 msec
	}
	env.pool = lb.NewClientPoolWithOptions(
		func(ctx context.Context, targets endpoint.Slice, mgmtScheme string) (lb.Client, error) {
			return fakeClientDial(ctx, &env, targets, mgmtScheme, clientOpts)
		},
		poolOpts,
	)
	defer env.pool.Close()

	numClusters := 100
	clusters := make([]endpoint.Slice, numClusters)
	for i := 0; i < len(clusters); i++ {
		port := 80
		if i%17 == 0 {
			port = blockForeverPort
		} else if i%19 == 0 {
			port = failPromptlyPort
		}
		clusters[i] = mkTargets(fmt.Sprintf("1.0.0.%d:%d", i, port))
	}

	rootCtx, cancel := context.WithTimeout(context.Background(), testTime)
	defer cancel()

	numFuzzers := 1000
	failed := make(chan struct{}, numFuzzers)
	var wg sync.WaitGroup
	wg.Add(numFuzzers)
	for i := 0; i < numFuzzers; i++ {
		go func() {
			defer wg.Done()
			fuzzUser(rootCtx, &env, clusters, failed)
		}()
	}

	select {
	case <-rootCtx.Done():
	case <-failed:
		cancel()
	}
clearFailures:
	for {
		select {
		case <-failed:
		default:
			break clearFailures
		}
	}
	wg.Wait()

	live := atomic.LoadInt64(&env.numClients)
	time.Sleep((poolOpts.LingerTime +
		time.Duration(clientOpts.maxDialDelayUsec)*time.Microsecond +
		time.Duration(clientOpts.maxOpDelayUsec)*time.Microsecond) * 2)
	t.Logf("clients: overall created: %d, live at test end: %d, live after linger: %d",
		atomic.LoadInt64(&env.totalClients), live, atomic.LoadInt64(&env.numClients))
}
