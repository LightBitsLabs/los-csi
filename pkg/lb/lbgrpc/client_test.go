// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

//go:build have_net && have_lb
// +build have_net,have_lb

package lbgrpc_test

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	guuid "github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/lightbitslabs/los-csi/pkg/lb"
	"github.com/lightbitslabs/los-csi/pkg/lb/lbgrpc"
	"github.com/lightbitslabs/los-csi/pkg/util/endpoint"
	"github.com/lightbitslabs/los-csi/pkg/util/strlist"
	"github.com/lightbitslabs/los-csi/pkg/util/wait"
)

const (
	defaultTimeout = 10 * time.Second
	GiB            = 1024 * 1024 * 1024

	// currently, LightOS is supposed to always round up the capacity to
	// the nearest 4KiB:
	lbCapGran = 4 * 1024

	doBlock   = true
	dontBlock = false

	exists      = true
	doesntExist = false

	doAdd = true
	doDel = false

	doCompress   = true
	dontCompress = false

	failCap = math.MaxInt64

	defProj = "default"
)

var (
	bogusUUID    = guuid.MustParse("deadbeef-e54c-4216-9456-3068e19e0b26")
	allowNoneACL = []string{lb.ACLAllowNone}
	defACL       = allowNoneACL
	prng         = rand.New(rand.NewSource(time.Now().UnixNano()))

	// flags:
	addrs           string // flag only, use `targets` in the code instead!
	clusterInfoPath string // path to JSON including cluster info
	logPath         string // path to file to store the log, '-' for stderr.
	jwtPath         string // path to JWT token to use for authN to LightOS API.

	log     = logrus.New()
	cluster *clusterInfo   // if nil - no cluster info JSON specified
	targets endpoint.Slice // filled in from `addrs` or `cluster`
	jwt     string         // contents of file at `jwtPath`
)

func initFlags() {
	flag.StringVar(&addrs, "lb-addrs", "",
		"comma-separated list of LB mgmt endpoints of the form: "+
			"<addr>:<port>[,<addr>:<port>...]")
	flag.StringVar(&clusterInfoPath, "cluster-info-path", "",
		"path to JSON file with cluster info and topology")
	flag.StringVar(&logPath, "log-path", "",
		"if empty: disables log, if '-': logs to stderr, otherwise: log "+
			"will be stored in a file at this path")
	flag.StringVar(&jwtPath, "jwt-path", "",
		"path to file with LightOS API auth JWT")

	flag.Parse()
}

func flagDie(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "SETUP ERROR: "+format+"\n", args...)
	fmt.Fprintf(os.Stderr, "             "+
		"run 'go test -args -h' for usage instructions\n")
	os.Exit(2)
}

func TestMain(m *testing.M) {
	initFlags()

	var err error
	if addrs != "" {
		if clusterInfoPath != "" {
			flagDie("only one of the flags -lb-addrs and -cluster-info-path" +
				"can be specified at a time, not both")
		}
		targets, err = endpoint.ParseCSV(addrs)
		if err != nil {
			flagDie("invalid -lb-addrs flag value '%s': %s", addrs, err)
		}
	} else if clusterInfoPath != "" {
		cluster = parseClusterInfo(clusterInfoPath)
		targets = make(endpoint.Slice, len(cluster.Nodes))
		for i, node := range cluster.Nodes {
			targets[i], err = endpoint.ParseStricter(node.MgmtEP)
			if err != nil {
				flagDie("bad cluster info file: invalid mgmt EP '%s' found: %s",
					node.MgmtEP, err)
			}
		}
	} else {
		flagDie("either valid LB mgmt endpoint must be specified using " +
			"-lb-addrs flag, or a full cluster topology in JSON " +
			"format must be passed in using -cluster-info-path flag")
	}
	if jwtPath != "" {
		jwt = loadJwt(jwtPath)
	} else {
		flagDie("path to valid 'system:cluster-admin' role JWT for authentication " +
			"to the cluster mgmt endpoint must be specified")
	}
	if logPath == "" {
		log.SetOutput(ioutil.Discard)
		log.SetLevel(logrus.PanicLevel)
	} else {
		log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02T15:04:05.000000-07:00",
		})
		log.SetLevel(logrus.DebugLevel)
		if logPath != "-" {
			f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			if err != nil {
				flagDie("failed to create log file '%s': %s", logPath, err)
			}
			defer func() {
				f.Sync()
				f.Close()
			}()
			log.SetOutput(f)
		}
	}

	os.Exit(m.Run())
}

type nodeInfo struct {
	Name     string
	Hostname string
	UUID     guuid.UUID
	MgmtEP   string
	DataEP   string
}

// for sort.Interface:
type nodeInfosByName []nodeInfo

func (n nodeInfosByName) Len() int           { return len(n) }
func (n nodeInfosByName) Swap(i, j int)      { n[i], n[j] = n[j], n[i] }
func (n nodeInfosByName) Less(i, j int) bool { return n[i].Name < n[j].Name }

type clusterInfo struct {
	UUID   guuid.UUID
	SubNQN string
	Nodes  []nodeInfo
}

// chkZeroFields aborts the test if struct passed in has zero (default)
// value in any of its fields.
func chkZeroFields(s interface{}) {
	v := reflect.ValueOf(s)
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).IsZero() {
			// get at the struct type, rather than the struct itself:
			st := reflect.Indirect(reflect.ValueOf(s)).Type()
			flagDie("bad cluster info file: no valid '%s.%s' field value found",
				st.Name(), st.Field(i).Name)
		}
	}
}

// parseClusterInfo attempts to read a cluster info JSON file from the
// specified path, parse it, and return a clusterInfo representation.
// if the process fails for any reason at any point - the function aborts
// the test with a corresponding error message.
//
// the expected JSON file format, by example, is:
//   {
//     "UUID": "864eb8af-f055-4c41-b8a3-e9a527483db3",
//     "SubNQN": "nqn.2014-08.org.nvmexpress:NVMf:uuid:c3dc6852-adcf-46f0-bc7b-a2d687ecee3a",
//     "Nodes": [
//       {
//         "Name": "lb-node-00",
//         "Hostname": "lb-node-00",
//         "UUID": "3f5e5352-3e29-4cea-984d-dbbe768a9476",
//         "MgmtEP": "172.18.0.7:80",
//         "DataEP": "172.18.0.7:4420"
//       },
//           ...
//     ]
//   }
func parseClusterInfo(path string) *clusterInfo {
	cij, err := ioutil.ReadFile(path)
	if err != nil {
		flagDie("invalid cluster info file path '%s' specified: %s", path, err)
	}
	var ci clusterInfo
	if err = json.Unmarshal(cij, &ci); err != nil {
		// not ideal, as errors here are vague, with no field names or
		// place in the hierarchy even when a problem is with a specific
		// field...
		flagDie("failed to parse cluster info file '%s': %s", path, err)
	}

	if len(ci.Nodes) < 1 {
		flagDie("bad cluster info file: no cluster nodes found")
	}
	chkZeroFields(ci)
	for _, node := range ci.Nodes {
		chkZeroFields(node)
	}

	return &ci
}

func loadJwt(path string) string {
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		flagDie("invalid JWT file path '%s' specified: %s", path, err)
	}
	return strings.TrimSpace(string(buf))
}

func getCtxWithJwt(timeout time.Duration) context.Context {
	ctx, _ := context.WithTimeout(context.Background(), timeout)
	return metadata.AppendToOutgoingContext(ctx, "Authorization", "Bearer "+jwt)
}

func getCtx() context.Context {
	return getCtxWithJwt(defaultTimeout)
}

func mkClient(t *testing.T) *lbgrpc.Client {
	clnt, err := lbgrpc.Dial(getCtx(), log.WithField("test", t.Name()), targets, "grpcs")
	if err != nil {
		t.Fatalf("BUG: Dial(%s) failed: '%s'", targets, err)
	}
	return clnt
}

func mkVolName() string {
	return fmt.Sprintf("lb-csi-ut-%08x", prng.Uint32())
}

// resource wrappers & ID: ---------------------------------------------------

type resID struct {
	name *string
	uuid *guuid.UUID
}

func byName(name string) resID     { return resID{name: &name} }
func byUUID(uuid guuid.UUID) resID { return resID{uuid: &uuid} }

func (r resID) byName() bool {
	if r.name != nil && r.uuid != nil {
		panic(fmt.Sprintf("FAIL: test error, resID has both name '%s' and UUID '%s', "+
			"but they're mutually exclusive", *r.name, *r.uuid))
	} else if r.name == nil && r.uuid == nil {
		panic("FAIL: test error, resID has neither name nor UUID, exactly one is required")
	}
	return r.name != nil
}

func (r resID) String() string {
	if r.byName() {
		return *r.name
	}
	return r.uuid.String()
}

type lbResource interface {
	kind() string
	getFn(rid resID) string
	secID(rid resID) string
}

type Volume struct {
	*lb.Volume
}

func (v Volume) String() string {
	return fmt.Sprintf("%+v", v.Volume)
}

func (v Volume) GoString() string {
	return fmt.Sprintf("%#v", v.Volume)
}

func (v Volume) kind() string {
	return "volume"
}

func (v Volume) getFn(rid resID) string {
	if rid.byName() {
		return fmt.Sprintf("GetVolumeByName(%s)", rid)
	}
	return fmt.Sprintf("GetVolume(%s)", rid)
}

func (v Volume) secID(rid resID) string {
	if rid.byName() {
		return v.UUID.String()
	}
	return v.Name
}

type Snapshot struct {
	*lb.Snapshot
}

func (s Snapshot) String() string {
	return fmt.Sprintf("%+v", s.Snapshot)
}

func (s Snapshot) GoString() string {
	return fmt.Sprintf("%#v", s.Snapshot)
}

func (s Snapshot) kind() string {
	return "volume"
}

func (s Snapshot) getFn(rid resID) string {
	if rid.byName() {
		return fmt.Sprintf("GetSnapshotByName(%s)", rid)
	}
	return fmt.Sprintf("GetSnapshot(%s)", rid)
}

func (s Snapshot) secID(rid resID) string {
	if rid.byName() {
		return s.UUID.String()
	}
	return s.Name
}

// tests: --------------------------------------------------------------------

func TestDial(t *testing.T) {
	clnt := mkClient(t)
	clntTgts := clnt.Targets()
	expTgts := targets.String()
	if clntTgts != expTgts {
		t.Fatalf("BUG: wrong Targets() after Dial():\nEXP: %s\nGOT: %s", expTgts, clntTgts)
	} else if testing.Verbose() {
		t.Logf("OK: Dail(%s) went ok, client reported targets: %s", targets, clntTgts)
	}
	clnt.Close()
}

func TestBadDial(t *testing.T) {
	tcs := []endpoint.Slice{
		{},
		{endpoint.EP{}},
		{endpoint.MustParse(guuid.New().String() + ".com:80")},
		// a bit of a hack. port 47 is currently reserved and unassigned, but...
		{endpoint.MustParse("localhost:47")},
	}

	for _, tc := range tcs {
		t.Run(tc.String(), func(t *testing.T) {
			ctx := getCtxWithJwt(3 * time.Second)
			clnt, err := lbgrpc.Dial(ctx, log.WithField("test", t.Name()), tc, "grpcs")
			if err == nil || clnt != nil {
				t.Fatalf("BUG: Dial(%s) succeeded on bogus target", tc)
			} else {
				t.Logf("OK: Dial(%s) failed with: '%s'", tc, err)
			}
		})
	}
}

func TestRemoteOk(t *testing.T) {
	clnt := mkClient(t)

	err := clnt.RemoteOk(getCtx())
	if err != nil {
		t.Errorf("BUG: RemoteOk() failed with: '%s'", err)
	} else {
		t.Logf("OK: RemoteOk() succeeded")
	}

	clnt.Close()
	err = clnt.RemoteOk(getCtx())
	if err == nil {
		t.Errorf("BUG: RemoteOk() succeeded on closed client")
	} else {
		t.Logf("OK: RemoteOk() failed with: '%s'", err)
	}
}

func TestClientID(t *testing.T) {
	clnt1 := mkClient(t)
	clnt2 := mkClient(t)
	if clnt1 == clnt2 {
		t.Fatalf("BUG: Dial(%s) returned the same object twice", targets)
	}
	id1a := clnt1.ID()
	id2a := clnt2.ID()
	if id1a == id2a {
		t.Fatalf("BUG: got clients with same ID '%s' from Dial(%s) invoked twice",
			id1a, targets)
	} else {
		t.Logf("OK: Dial(%s) called twice returned clients with IDs: '%s', '%s'",
			targets, id1a, id2a)
	}

	for i, clnt := range []*lbgrpc.Client{clnt1, clnt2} {
		err := clnt.RemoteOk(getCtx())
		if err != nil {
			t.Errorf("BUG: clnt%d.RemoteOk() failed with: '%s'", i, err)
		} else {
			t.Logf("OK: clnt%d.RemoteOk() succeeded", i)
		}
	}
	id1b := clnt1.ID()
	id2b := clnt2.ID()
	if id1a != id1b || id2a != id2b {
		t.Fatalf("BUG: client IDs changed after calls:\nEXP: '%s', '%s'\nGOT: '%s', '%s'",
			id1a, id2a, id1b, id2b)
	} else {
		t.Logf("OK: client IDs remained the same after calls: '%s', '%s'", id1a, id2a)
	}

	clnt1.Close()
	clnt2.Close()
}

func testGetGlobals(t *testing.T, exp *clusterInfo) {
	clnt := mkClient(t)
	defer clnt.Close()

	cl, err := clnt.GetCluster(getCtx())
	if err != nil {
		t.Fatalf("BUG: GetCluster() failed with: '%s'", err)
	} else {
		t.Logf("OK: GetCluster() got a cluster")
	}
	if exp != nil && cl.UUID != exp.UUID {
		t.Errorf("BUG: GetCluster() got unexpected UUID '%s' != '%s'",
			cl.UUID, exp.UUID)
	} else {
		t.Logf("OK: GetCluster() got cluster with UUID '%s'", cl.UUID)
	}
	if exp != nil && cl.SubsysNQN != exp.SubNQN {
		t.Errorf("BUG: GetCluster() got unexpected SubNQN '%s' != '%s'",
			cl.SubsysNQN, exp.SubNQN)
	} else {
		t.Logf("OK: GetCluster() got cluster with SubNQN '%s'", cl.SubsysNQN)
	}

	nodes, err := clnt.ListNodes(getCtx())
	numNodes := len(nodes)
	if err != nil {
		t.Fatalf("BUG: ListNodes() failed with: '%s'", err)
	} else {
		t.Logf("OK: ListNodes() got %d nodes", numNodes)
	}

	numExpNodes := 0
	if exp != nil {
		numExpNodes = len(exp.Nodes)
		sort.Sort(nodeInfosByName(exp.Nodes))
	}
	sort.Sort(lb.NodesByName(nodes))
	if exp != nil && numNodes != numExpNodes {
		realNames := make([]string, 0, numNodes)
		for _, node := range nodes {
			realNames = append(realNames, node.Name)
		}
		expNames := make([]string, 0, numExpNodes)
		for _, node := range exp.Nodes {
			expNames = append(expNames, node.Name)
		}
		t.Fatalf("BUG: ListNodes() and cluster info file node lists differ "+
			"(%d vs %d):\n%q\n%q", numNodes, numExpNodes, realNames, expNames)
	}

	if exp == nil {
		return
	}
	for i, node := range nodes {
		ok := true
		expNode := exp.Nodes[i]
		if node.Name != exp.Nodes[i].Name {
			t.Errorf("BUG: ListNodes()[%d] botched node name: '%s' != '%s",
				i, node.Name, expNode.Name)
			ok = false
		}
		if node.UUID != expNode.UUID {
			t.Errorf("BUG: ListNodes()[%d] botched '%s' UUID: '%s' != '%s'",
				i, node.Name, node.UUID, expNode.UUID)
			ok = false
		}
		if node.DataEP.String() != expNode.DataEP {
			t.Errorf("BUG: ListNodes()[%d] botched '%s' data EP '%s' != '%s'",
				i, node.Name, node.DataEP.String(), expNode.DataEP)
			ok = false
		}
		if node.HostName != expNode.Hostname {
			t.Errorf("BUG: ListNodes()[%d] botched '%s' hostname '%s' != '%s'",
				i, node.Name, node.HostName, expNode.Hostname)
			ok = false
		}
		if ok {
			t.Logf("OK: ListNodes()[%d] got node '%s' right", i, node.Name)
		}
	}
}

func TestGetGlobals(t *testing.T) {
	if cluster == nil {
		t.Skip("skipping test: no cluster info JSON file path specified")
	}
	testGetGlobals(t, cluster)
}

func TestGetGlobalsNoVerify(t *testing.T) {
	if cluster != nil {
		t.Skip("skipping test: cluster info JSON file specified, " +
			"see TestGetGlobals() results instead")
	}
	testGetGlobals(t, cluster)
}

// TODO: "a little copying is better than a little dependency", but this
// isStatusNotFound() copying all over the place is getting ridiculous!
func isStatusNotFound(err error) bool {
	st, ok := status.FromError(err)
	if ok && st.Code() == codes.NotFound {
		return true
	}
	return false
}

func chkGetResult(
	t *testing.T, existing bool, rid resID, res lbResource, what string, err error,
) {
	method := res.getFn(rid)
	if existing {
		if err != nil {
			t.Fatalf("BUG: %s failed with: '%s'", method, err)
		} else {
			t.Logf("OK: %s got %s: %s", method, res.kind(), res.secID(rid))
		}
	} else {
		if err == nil {
			t.Fatalf("BUG: %s succeeded despite %s:\n%+v",
				method, what, res)
		} else if isStatusNotFound(err) {
			t.Logf("OK: %s failed on %s with 'NotFound'", method, what)
		} else {
			t.Fatalf("BUG: %s failed on %s with: '%s'", method, what, err)
		}
	}
}

func getLbVol(clnt *lbgrpc.Client, rid resID) (Volume, error) {
	var lbVol *lb.Volume
	var err error
	if rid.byName() {
		lbVol, err = clnt.GetVolumeByName(getCtx(), *rid.name, defProj)
	} else {
		lbVol, err = clnt.GetVolume(getCtx(), *rid.uuid, defProj)
	}
	return Volume{lbVol}, err
}

func getVolIfExistOrFatal(
	t *testing.T, clnt *lbgrpc.Client, rid resID, existing bool, what string,
) Volume {
	vol, err := getLbVol(clnt, rid)
	chkGetResult(t, existing, rid, vol, what, err)
	return vol
}

func getNoVolOrFatal(
	t *testing.T, clnt *lbgrpc.Client, rid resID, what string,
) {
	_ = getVolIfExistOrFatal(t, clnt, rid, doesntExist, what)
}

const (
	skipNone = 0
	skipName = (1 << iota)
	skipUUID
	skipCapacity
	skipSnapUUID
	skipETag
)

func chkVolEquals(t *testing.T, l, r Volume, lDescr, rDescr string, skipFields uint32) {
	do := func(field uint32) bool { return skipFields&field == field }

	mockLBVol := *l.Volume
	mockL := Volume{&mockLBVol}
	if do(skipName) {
		mockL.Name = r.Name
	}
	if do(skipUUID) {
		mockL.UUID = r.UUID
	}
	if do(skipCapacity) {
		mockL.Capacity = r.Capacity
	}
	if do(skipSnapUUID) {
		mockL.SnapshotUUID = r.SnapshotUUID
	}
	if do(skipETag) {
		mockL.ETag = r.ETag
	}

	if !reflect.DeepEqual(mockL, r) {
		t.Errorf("BUG: %s and %s differ:\n%+v\n%+v",
			lDescr, rDescr, l, r)
	} else {
		t.Logf("OK: %s and %s match", lDescr, rDescr)
	}
}

func getVolEqualsOrFatal(
	t *testing.T, clnt *lbgrpc.Client, rid resID, expVol Volume, skipFields uint32,
	expVolOrigin string,
) Volume {
	vol := getVolIfExistOrFatal(t, clnt, rid, exists, expVolOrigin)
	chkVolEquals(t, expVol, vol, expVolOrigin, fmt.Sprintf("%s results", vol.getFn(rid)), skipFields)
	return vol
}

func delVolIfExistOrFatal(
	t *testing.T, clnt *lbgrpc.Client, uuid guuid.UUID, blocking bool, existing bool,
) {
	// in some envs (e.g. InstaCluster), kicking off an async delete of an
	// empty volume can sometimes take a while:
	delCtx := getCtxWithJwt(10 * time.Second)

	err := clnt.DeleteVolume(delCtx, uuid, defProj, blocking)
	if existing {
		if err != nil {
			t.Fatalf("BUG: DeleteVolume(%s) failed with: '%s'", uuid, err)
		} else {
			t.Logf("OK: DeleteVolume(%s) reported success", uuid)
		}
	} else {
		if err == nil {
			t.Errorf("BUG: DeleteVolume(%s) succeeded despite bogus volume UUID",
				uuid)
		} else if isStatusNotFound(err) {
			t.Logf("OK: DeleteVolume(%s) failed with 'NotFound'", uuid)
		} else {
			t.Errorf("BUG: DeleteVolume(%s) failed with: '%s'", uuid, err)
		}
	}
}

func isPseudoACE(ace string) bool {
	return ace == lb.ACLAllowNone || ace == lb.ACLAllowAny
}

func aclOpToStr(add bool) string {
	if add {
		return "add"
	}
	return "delete"
}

func modVolACL(
	t *testing.T, clnt *lbgrpc.Client, uuid *guuid.UUID, vol *Volume, add bool, ace string,
	expACL []string,
) (*Volume, error) {
	updateMade := false // updated by the hooks

	addVolACEHook := func(vol *lb.Volume) (*lb.VolumeUpdate, error) {
		pseudoACE := false
		for _, a := range vol.ACL {
			if a == ace {
				return nil, nil
			}
			if isPseudoACE(a) {
				pseudoACE = true
			}
		}
		updateMade = true
		var acl []string
		if pseudoACE {
			if len(vol.ACL) != 1 {
				return nil, fmt.Errorf("got vol that mixes pseudo ACEs with real "+
					"ACEs: %#q", vol.ACL)
			}
			if vol.ACL[0] == lb.ACLAllowAny {
				return nil, fmt.Errorf("can't add ACE to ALLOW_ANY ACL")
			}
			acl = []string{ace}
		} else {
			acl = append(append(vol.ACL[:0:0], vol.ACL...), ace)
		}
		return &lb.VolumeUpdate{ACL: acl}, nil
	}

	delVolACEHook := func(vol *lb.Volume) (*lb.VolumeUpdate, error) {
		acl := make([]string, 0, len(vol.ACL))
		needsUpdate := false
		for _, a := range vol.ACL {
			if a == ace {
				needsUpdate = true
				updateMade = true
			} else {
				acl = append(acl, a)
			}
		}
		if !needsUpdate {
			return nil, nil
		}
		if len(acl) == 0 {
			acl = append(acl, lb.ACLAllowNone)
		}
		return &lb.VolumeUpdate{ACL: acl}, nil
	}

	hook := addVolACEHook
	if !add {
		hook = delVolACEHook
	}

	if uuid == nil {
		uuid = &vol.UUID
	}

	lbVol, err := clnt.UpdateVolume(getCtx(), *uuid, defProj, hook)
	if err != nil {
		return nil, err
	}
	if updateMade {
		if reflect.DeepEqual(vol, lbVol) {
			t.Errorf("BUG: UpdateVolume(%s) failed to change the volume", *uuid)
		}
		if vol.ETag == lbVol.ETag {
			t.Errorf("BUG: UpdateVolume(%s) failed to change the volume ETag", *uuid)
		}
	}

	newVol := Volume{lbVol}

	if expACL == nil {
		return &newVol, nil
	}

	opStr := aclOpToStr(add)
	if !strlist.AreEqual(expACL, newVol.ACL) {
		t.Errorf("BUG: UpdateVolume(%s) failed to %s ACE '%s':\nEXP: %#q\nGOT: %#q",
			*uuid, opStr, ace, expACL, newVol.ACL)
	} else {
		t.Logf("OK: UpdateVolume(%s) managed to %s ACE '%s': %#q",
			*uuid, opStr, ace, newVol.ACL)
	}

	return &newVol, nil
}

func modVolACLOrFatal(
	t *testing.T, clnt *lbgrpc.Client, vol Volume, add bool, ace string, expACL []string,
) Volume {
	newVol, err := modVolACL(t, clnt, nil, &vol, add, ace, expACL)
	if err != nil {
		t.Fatalf("BUG: UpdateVolume(%s) to %s ACE '%s' from ACL %#q failed with: '%s'",
			vol.UUID, aclOpToStr(add), ace, vol.ACL, err)
	}
	return *newVol
}

func createVolOrFatal(
	t *testing.T, clnt *lbgrpc.Client, name string, size uint64, repCount uint32,
	compress bool, acl []string, proj string, snapUUID *guuid.UUID, blocking bool,
) Volume {
	base := ""
	baseUUID := guuid.Nil
	if snapUUID != nil {
		base = fmt.Sprintf("from snapshot %s ", *snapUUID)
		baseUUID = *snapUUID
	}
	vol, err := clnt.CreateVolume(getCtx(), name, size, repCount, compress, acl,
		proj, baseUUID, blocking)
	if err != nil {
		t.Fatalf("BUG: CreateVolume(%s) %sfailed with: '%s'", name, base, err)
	} else {
		t.Logf("OK: CreateVolume(%s) %screated volume UUID: %s", name, base, vol.UUID)
	}
	return Volume{vol}
}

// waitForDelVolToDisappearOrFatal() makes sure that the volume specified by `rid` (`name`
// XOR `uuid`) is either totally gone, or, if it still exists, that it's in the 'Deleting'
// state, and if so - waits up to ~42 sec for it to totally disappear.
//
// apparently the behaviour of the LB API changed again at some point, and by-name
// GetVolume() requests started returning volumes in the 'Deleting' state again...
func waitForDelVolToDisappearOrFatal(
	t *testing.T, clnt *lbgrpc.Client, rid resID, what string,
) {
	method := Volume{}.getFn(rid)
	t.Logf("OK: polling %s till volume totally disappears...", method)

	start := time.Now()

	// sometimes even empty volumes take some time to totally disappear,
	// especially in some envs (e.g. InstaCluster, overloaded VMs):
	opts := wait.Backoff{
		Delay:      100 * time.Millisecond,
		Factor:     2.0,
		DelayLimit: 2 * time.Second,
		Retries:    20,
	}
	err := wait.WithExponentialBackoff(opts, func() (bool, error) {
		vol, err := getLbVol(clnt, rid)
		infix := fmt.Sprintf("in %.1f sec", time.Now().Sub(start).Seconds())
		if err == nil {
			switch vol.State {
			case lb.VolumeDeleting:
				return false, nil
			default:
				return true, fmt.Errorf("BUG: %s succeeded %s %s, "+
					"but got vol in unexpected state '%s':\n%+v",
					method, what, infix, vol.State, vol)
			}
		} else if isStatusNotFound(err) {
			t.Logf("OK: %s failed %s with 'NotFound'", method, infix)
			return true, nil
		} else {
			return true, fmt.Errorf("BUG: %s failed %s with: '%s'",
				method, infix, err)
		}
	})
	if err != nil {
		t.Fatalf("BUG: failed polling %s: %s", method, err.Error())
	}
}

func delVolAndWaitForItOrFatal(t *testing.T, clnt *lbgrpc.Client, uuid guuid.UUID, what string) {
	delVolIfExistOrFatal(t, clnt, uuid, doBlock, exists)
	waitForDelVolToDisappearOrFatal(t, clnt, byUUID(uuid), what)
}

func TestVolume(t *testing.T) {
	clnt := mkClient(t)
	defer clnt.Close()

	var volCap uint64 = 4*1024*1024*1024 - 4096
	var numReplicas uint32 = 3
	volName := mkVolName()
	bogusName := fmt.Sprintf("lb-csi-ut-bogus-%08x", prng.Uint32())

	vol := createVolOrFatal(t, clnt, volName, volCap, numReplicas, dontCompress, nil,
		defProj, nil, doBlock)
	volUUID := vol.UUID
	if !strlist.AreEqual(defACL, vol.ACL) {
		t.Errorf("BUG: CreateVolume(%s) messed up ACL:\nEXP: %#q\nGOT: %#q",
			volName, defACL, vol.ACL)
	} else {
		t.Logf("OK: CreateVolume(%s) set default ACL workaround correctly", volName)
	}

	// LightOS rounds up capacity to lbCapGran (curr: 4KiB):
	if (volCap+lbCapGran-1)/lbCapGran*lbCapGran != vol.Capacity {
		t.Errorf("BUG: CreateVolume(%s) unexpected capacity: %d for %d request",
			volName, vol.Capacity, volCap)
	} else {
		t.Logf("OK: CreateVolume(%s) set capacity within expected bounds", volName)
	}

	getNoVolOrFatal(t, clnt, byName(bogusName), "bogus volume name")
	getNoVolOrFatal(t, clnt, byUUID(bogusUUID), "bogus volume UUID")

	vol2 := getVolEqualsOrFatal(t, clnt, byName(volName), vol, skipNone,
		fmt.Sprintf("CreateVolume(%s)", volName))
	// sanity check:
	vol2.Capacity = 123456
	if reflect.DeepEqual(vol, vol2) {
		t.Errorf("BUG: volume comparison code is borked, above results are suspect")
	} else {
		t.Logf("OK: volume comparison code is not obviously broken")
	}

	vol2 = getVolEqualsOrFatal(t, clnt, byUUID(volUUID), vol, skipNone,
		fmt.Sprintf("CreateVolume(%s)", volName))

	a1 := "ace-1"
	a2 := "ace-2"
	bogusVol, err := modVolACL(t, clnt, &bogusUUID, nil, doAdd, a1, defACL)
	if err == nil {
		t.Errorf("BUG: UpdateVolume(%s) succeeded despite bogus volume UUID:\n%+v",
			bogusUUID, bogusVol)
	} else if isStatusNotFound(err) {
		t.Logf("OK: UpdateVolume(%s) failed with 'NotFound'", bogusUUID)
	} else {
		t.Errorf("BUG: UpdateVolume(%s) failed with: '%s'", bogusUUID, err)
	}

	vol2 = modVolACLOrFatal(t, clnt, vol2, doAdd, lb.ACLAllowNone, defACL)
	vol2 = modVolACLOrFatal(t, clnt, vol2, doAdd, a2, []string{a2})
	vol2 = modVolACLOrFatal(t, clnt, vol2, doAdd, a1, []string{a1, a2})
	vol2 = modVolACLOrFatal(t, clnt, vol2, doAdd, a2, []string{a1, a2})
	vol2 = modVolACLOrFatal(t, clnt, vol2, doDel, a2, []string{a1})
	vol2 = modVolACLOrFatal(t, clnt, vol2, doDel, a2, []string{a1})
	vol2 = modVolACLOrFatal(t, clnt, vol2, doDel, a1, defACL)
	vol2 = modVolACLOrFatal(t, clnt, vol2, doDel, a1, defACL)
	vol2 = modVolACLOrFatal(t, clnt, vol2, doAdd, lb.ACLAllowAny, []string{lb.ACLAllowAny})
	vol2 = modVolACLOrFatal(t, clnt, vol2, doDel, lb.ACLAllowAny, defACL)

	// other than ETags the volume should have remained the same:
	chkVolEquals(t, vol, vol2, "volume as created", "after a series of self-cancelling ACL changes",
		skipETag)

	delVolIfExistOrFatal(t, clnt, bogusUUID, doBlock, doesntExist)

	delVolAndWaitForItOrFatal(t, clnt, volUUID, "on deleted volume")
	getNoVolOrFatal(t, clnt, byName(volName), "volume previously successfully deleted")
}

func TestVolumeAvailableAfterDelete(t *testing.T) {
	clnt := mkClient(t)
	defer clnt.Close()

	var volCap uint64 = 4 * 1024 * 1024 * 1024
	var numReplicas uint32 = 3
	volName := mkVolName()

	vol := createVolOrFatal(t, clnt, volName, volCap, numReplicas, dontCompress, nil,
		defProj, nil, dontBlock)
	uuid := vol.UUID
	getVolEqualsOrFatal(t, clnt, byUUID(uuid), vol, skipNone,
		fmt.Sprintf("CreateVolume(%s)", volName))
	delVolIfExistOrFatal(t, clnt, uuid, dontBlock, exists)

	tmo := 120 * time.Second // <sigh> sometimes the cluster is busy...
	if testing.Short() {
		tmo = 15 * time.Second
	}

	type stateStats struct {
		name string
		num  int
		time time.Duration
	}
	stats := []stateStats{}

	fmtStats := func(lim int) string {
		ents := []string{}
		if len(stats) < lim {
			lim = len(stats)
		}
		for _, s := range stats[:lim] {
			ents = append(ents, fmt.Sprintf("%-18s :%4d (%s)", s.name, s.num, s.time))
		}
		return strings.Join(ents, "\n")
	}

	start := time.Now()
	const gone = "<NON-EXISTENT>"
	const getError = "<GET-VOLUME-ERROR>"
	last := stateStats{gone, 0, 0}
	lastStart := start
	pushState := func(name string) {
		if name == last.name {
			last.num++
		} else {
			now := time.Now()
			last.time = now.Sub(lastStart)
			stats = append(stats, last)
			last = stateStats{name, 1, 0}
			lastStart = now
		}
	}

	for time.Now().Sub(start) < tmo {
		vol, err := clnt.GetVolume(getCtx(), uuid, defProj)
		if err == nil {
			pushState(vol.State.String())
		} else if isStatusNotFound(err) {
			pushState(gone)
		} else {
			pushState(fmt.Sprintf("%s: %s>", getError, err))
		}
	}
	// flush last state and drop 1st placeholder:
	pushState("")
	if stats[0].num == 0 {
		stats = stats[1:]
	}

	nstats := len(stats)
	if stats[nstats-1].name == gone {
		if nstats == 1 {
			t.Logf("OK: after DeleteVolume() succeeded - volume disappeared")
		} else if nstats == 2 && stats[0].name == lb.VolumeDeleting.String() {
			t.Logf("OK: after DeleteVolume() succeeded, volume spent %s in "+
				"state '%s' then disappeared", stats[0].time, stats[0].name)
		} else {
			t.Fatalf("BUG: after DeleteVolume() success, volume flipped %d states "+
				"in %s, then disappeared:\n%s", nstats-1, tmo, fmtStats(20))
		}
	} else {
		t.Fatalf("BUG: after DeleteVolume() success, volume remained in state '%s' after "+
			"flipping %d states in %s:\n%s",
			stats[nstats-1].name, nstats, tmo, fmtStats(20))
	}
}

func TestLBVolumeCreateSizeHandling(t *testing.T) {
	clnt := mkClient(t)
	defer clnt.Close()

	type capacities struct {
		ask  uint64
		want uint64
	}

	roundUp := func(c uint64) capacities {
		if c == 0 || c == math.MaxInt64 {
			return capacities{c, failCap}
		}
		return capacities{c, (c + lbCapGran - 1) / lbCapGran * lbCapGran}
	}
	fiver := func(c uint64) []capacities {
		return []capacities{
			roundUp(c - 4096),
			roundUp(c - 1),
			roundUp(c),
			roundUp(c + 1),
			roundUp(c + 4096),
		}
	}

	testCases := fiver(1024 * 1024 * 1024)

	if !testing.Short() {
		shorties := []capacities{
			// these can't be generated with fiver():
			roundUp(0),
			roundUp(511),
			roundUp(512),
			roundUp(4095),
			roundUp(4096),
		}
		testCases = append(testCases, shorties...)
		fivers := [][]capacities{
			fiver(1 * 1024 * 1024),
			fiver(512 * 1024 * 1024),
			fiver(1536 * 1024 * 1024),
			fiver(2 * 1024 * 1024 * 1024),
			fiver(3 * 1024 * 1024 * 1024),
		}
		for _, f := range fivers {
			testCases = append(testCases, f...)
		}
	}

	cemetery := []guuid.UUID{}

	for _, tt := range testCases {
		t.Run(fmt.Sprintf("%d-%d", tt.ask, tt.want), func(t *testing.T) {
			uuid := testLBVolumeCreateSizeHandling(t, clnt, tt.ask, tt.want)
			if uuid != guuid.Nil {
				cemetery = append(cemetery, uuid)
			}
		})
	}

	// this is a bit of an odd heuristics hack:
	for _, uuid := range cemetery[0 : len(cemetery)-2] {
		delVolIfExistOrFatal(t, clnt, uuid, dontBlock, exists)
	}
	delVolIfExistOrFatal(t, clnt, cemetery[len(cemetery)-1], doBlock, exists)
	time.Sleep(time.Duration(len(cemetery)*50) * time.Millisecond)
	for _, uuid := range cemetery[0 : len(cemetery)-2] {
		delVolIfExistOrFatal(t, clnt, uuid, dontBlock, doesntExist)
	}
}

func testLBVolumeCreateSizeHandling(
	t *testing.T, clnt *lbgrpc.Client, askCap uint64, wantCap uint64,
) guuid.UUID {
	var numReplicas uint32 = 2
	volName := mkVolName()

	// some versions of LB returned different capacities on CreateVolume() and
	// GetVolume(), so grab the results manually and separately:
	vol, err := clnt.CreateVolume(getCtx(), volName, askCap, numReplicas, dontCompress, nil,
		defProj, guuid.Nil, dontBlock)
	if err != nil {
		if wantCap == failCap {
			t.Logf("OK: CreateVolume(%s) failed on bogus capacity %d", volName, askCap)
		} else {
			t.Errorf("BUG: CreateVolume(%s) of %d bytes failed with: '%s'",
				volName, askCap, err)
		}
		return guuid.Nil
	}

	chkCap := func(vol *lb.Volume, method string) {
		// LightOS-side granularity validation got disabled at some point,
		// so currently LB creates volumes of EXACTLY the requested capacity...
		if vol.Capacity != wantCap {
			t.Errorf("BUG: %s(%s) of %d bytes got vol of %d instead of %d",
				method, volName, askCap, vol.Capacity, wantCap)
		} else if testing.Verbose() {
			rounded := ""
			if askCap != wantCap {
				rounded = " (rounded up)"
			}
			t.Logf("OK: %s(%s) of %d bytes got vol of %d%s",
				method, volName, askCap, wantCap, rounded)
		}
	}
	chkCap(vol, "CreateVolume")

	// now manually wait for it to become available:
	time.Sleep(200 * time.Millisecond)
	opts := wait.Backoff{Delay: 100 * time.Millisecond, Retries: 15}
	err = wait.WithExponentialBackoff(opts, func() (bool, error) {
		getVol, err := clnt.GetVolume(getCtx(), vol.UUID, defProj)
		if err != nil {
			return false, err
		}

		chkCap(getVol, "GetVolume")

		switch getVol.State {
		case lb.VolumeCreating:
			// play it again, Sam...
			return false, nil
		case lb.VolumeAvailable:
			return true, nil
		default:
			return false, fmt.Errorf("BUG: GetVolume(%s) result is in unexpected "+
				"state %s (%d) while waiting for it to be created",
				vol.UUID, getVol.State, getVol.State)
		}
	})
	if err != nil {
		t.Errorf(err.Error())
		return guuid.Nil
	}
	return vol.UUID
}

func createSnapOrFatal(
	t *testing.T, clnt *lbgrpc.Client, name string, projectName string, srcVolUUID guuid.UUID,
	descr string, blocking bool,
) Snapshot {
	snap, err := clnt.CreateSnapshot(getCtx(), name, defProj, srcVolUUID, descr, doBlock)
	if err != nil {
		t.Fatalf("BUG: CreateSnapshot(%s) failed with: '%s'", name, err)
	} else {
		t.Logf("OK: CreateSnapshot(%s) created snapshot UUID: %s", name, snap.UUID)
	}
	return Snapshot{snap}
}

func getLbSnap(clnt *lbgrpc.Client, rid resID) (Snapshot, error) {
	var lbSnap *lb.Snapshot
	var err error
	if rid.byName() {
		lbSnap, err = clnt.GetSnapshotByName(getCtx(), *rid.name, defProj)
	} else {
		lbSnap, err = clnt.GetSnapshot(getCtx(), *rid.uuid, defProj)
	}
	return Snapshot{lbSnap}, err
}

func getSnapIfExistOrFatal(
	t *testing.T, clnt *lbgrpc.Client, rid resID, existing bool, what string,
) Snapshot {
	snap, err := getLbSnap(clnt, rid)
	chkGetResult(t, existing, rid, snap, what, err)
	return snap
}

func getNoSnapOrFatal(
	t *testing.T, clnt *lbgrpc.Client, rid resID, what string,
) {
	_ = getSnapIfExistOrFatal(t, clnt, rid, doesntExist, what)
}

func chkSnapEquals(t *testing.T, l, r Snapshot, lDescr, rDescr string) {
	if !reflect.DeepEqual(l, r) {
		t.Errorf("BUG: %s and %s results differ:\n%+v\n%+v",
			lDescr, rDescr, l, r)
	} else {
		t.Logf("OK: %s and %s results match", lDescr, rDescr)
	}
}

func getSnapEqualsOrFatal(
	t *testing.T, clnt *lbgrpc.Client, rid resID, expSnap Snapshot, expSnapOrigin string,
) Snapshot {
	snap := getSnapIfExistOrFatal(t, clnt, rid, exists, expSnapOrigin)
	chkSnapEquals(t, expSnap, snap, expSnapOrigin, snap.getFn(rid))
	return snap
}

func delSnapIfExistOrFatal(
	t *testing.T, clnt *lbgrpc.Client, uuid guuid.UUID, blocking bool, existing bool,
) {
	// in some envs (e.g. InstaCluster), kicking off an async delete of an
	// empty snapshot can sometimes take a while:
	delCtx := getCtxWithJwt(10 * time.Second)

	err := clnt.DeleteSnapshot(delCtx, uuid, defProj, blocking)
	if existing {
		if err != nil {
			t.Fatalf("BUG: DeleteSnapshot(%s) failed with: '%s'", uuid, err)
		} else {
			t.Logf("OK: DeleteSnapshot(%s) reported success", uuid)
		}
	} else {
		if err == nil {
			t.Errorf("BUG: DeleteSnapshot(%s) succeeded despite bogus snapshot UUID",
				uuid)
		} else if isStatusNotFound(err) {
			t.Logf("OK: DeleteSnapshot(%s) failed with 'NotFound'", uuid)
		} else {
			t.Errorf("BUG: DeleteSnapshot(%s) failed with: '%s'", uuid, err)
		}
	}
}

// waitForDelSnapToDisappearOrFatal() makes sure that the snapshot specified by `rid`
// (`name` XOR `uuid`) is either totally gone, or, if it still exists, that it's in the
// 'Deleting' state, and if so - waits up to ~2 min for it to totally disappear.
//
// Go generics, where art thou?!?
func waitForDelSnapToDisappearOrFatal(
	t *testing.T, clnt *lbgrpc.Client, rid resID, what string,
) {
	method := Snapshot{}.getFn(rid)
	t.Logf("OK: polling %s till snapshot totally disappears...", method)

	start := time.Now()

	// sometimes even empty snapshots take some time to totally disappear,
	// especially in some envs (e.g. InstaCluster, overloaded VMs):
	opts := wait.Backoff{
		Delay:      100 * time.Millisecond,
		Factor:     2.0,
		DelayLimit: 2 * time.Second,
		Retries:    60,
	}
	err := wait.WithExponentialBackoff(opts, func() (bool, error) {
		snap, err := getLbSnap(clnt, rid)
		infix := fmt.Sprintf("in %.1f sec", time.Now().Sub(start).Seconds())
		if err == nil {
			switch snap.State {
			case lb.SnapshotDeleting:
				return false, nil
			default:
				return true, fmt.Errorf("BUG: %s succeeded %s %s, "+
					"but got vol in unexpected state '%s':\n%+v",
					method, what, infix, snap.State, snap)
			}
		} else if isStatusNotFound(err) {
			t.Logf("OK: %s failed %s with 'NotFound'", method, infix)
			return true, nil
		} else {
			return true, fmt.Errorf("BUG: %s failed %s with: '%s'",
				method, infix, err)
		}
	})
	if err != nil {
		t.Fatalf("BUG: failed polling %s: %s", method, err.Error())
	}
}

func delSnapAndWaitForItOrFatal(t *testing.T, clnt *lbgrpc.Client, uuid guuid.UUID, what string) {
	delSnapIfExistOrFatal(t, clnt, uuid, doBlock, exists)
	waitForDelSnapToDisappearOrFatal(t, clnt, byUUID(uuid), what)
}

func TestSnapshot(t *testing.T) {
	clnt := mkClient(t)
	defer clnt.Close()

	var volCap uint64 = 4*1024*1024*1024 - 4096
	var numReplicas uint32 = 2
	volName := mkVolName()
	bogusName := fmt.Sprintf("lb-csi-ut-bogus-%08x", prng.Uint32())

	vol := createVolOrFatal(t, clnt, volName, volCap, numReplicas, doCompress, nil,
		defProj, nil, doBlock)

	snapName := mkVolName()
	descr := guuid.New().String()
	expSnap := Snapshot{&lb.Snapshot{
		Name:               snapName,
		Capacity:           volCap,
		State:              lb.SnapshotAvailable,
		SrcVolUUID:         vol.UUID,
		SrcVolName:         volName,
		SrcVolReplicaCount: numReplicas,
		SrcVolCompression:  doCompress,
		ProjectName:        defProj,
	}}

	snap := createSnapOrFatal(t, clnt, snapName, defProj, vol.UUID, descr, doBlock)
	snapUUID := snap.UUID
	expSnap.UUID = snapUUID
	expSnap.CreationTime = snap.CreationTime
	chkSnapEquals(t, snap, expSnap, fmt.Sprintf("CreateSnapshot(%s)", snapName), "expected")

	getNoSnapOrFatal(t, clnt, byName(snapUUID.String()), "snap UUID passed as name")
	getNoSnapOrFatal(t, clnt, byName(bogusName), "bogus snap name")
	getNoSnapOrFatal(t, clnt, byUUID(bogusUUID), "bogus snap UUID")

	snap2 := getSnapEqualsOrFatal(t, clnt, byName(snapName), snap,
		fmt.Sprintf("after CreateSnapshot(%s)", snapName))
	// sanity check:
	snap2.Capacity = 123456
	if reflect.DeepEqual(snap, snap2) {
		t.Errorf("BUG: snapshot comparison code is borked, above results are suspect")
	} else {
		t.Logf("OK: snapshot comparison code is not obviously broken")
	}
	snap2 = getSnapEqualsOrFatal(t, clnt, byUUID(snapUUID), snap,
		fmt.Sprintf("after CreateSnapshot(%s)", snapName))

	delSnapIfExistOrFatal(t, clnt, bogusUUID, doBlock, doesntExist)

	delSnapAndWaitForItOrFatal(t, clnt, snapUUID, "on deleted snapshot")
	getNoSnapOrFatal(t, clnt, byName(snapName), "previously successfully deleted")

	delVolIfExistOrFatal(t, clnt, bogusUUID, doBlock, doesntExist)

	delVolAndWaitForItOrFatal(t, clnt, vol.UUID, "on deleted volume")
	getNoVolOrFatal(t, clnt, byName(volName), "volume previously successfully deleted")
}

func TestClone(t *testing.T) {
	clnt := mkClient(t)
	defer clnt.Close()

	var volCap uint64 = 4*1024*1024*1024 - 4096
	var numReplicas uint32 = 2
	volName := mkVolName()

	vol := createVolOrFatal(t, clnt, volName, volCap, numReplicas, doCompress, nil,
		defProj, nil, doBlock)

	snapName := mkVolName()
	descr := guuid.New().String()
	snap := createSnapOrFatal(t, clnt, snapName, defProj, vol.UUID, descr, doBlock)

	cloneName := mkVolName()
	clone := createVolOrFatal(t, clnt, cloneName, volCap, numReplicas, doCompress, nil,
		defProj, &snap.UUID, doBlock)

	if clone.Name != cloneName || clone.UUID == vol.UUID {
		t.Fatalf("BUG: original volume is TOO similar to the clone:\n%+v\n%+v", vol, clone)
	}
	if clone.SnapshotUUID != snap.UUID {
		t.Fatalf("BUG: clone is not based on the original volume:\n%+v", clone)
	}
	chkVolEquals(t, vol, clone, "original volume", "its clone", skipName|skipUUID|skipSnapUUID)

	snap2Name := mkVolName()
	descr2 := guuid.New().String()
	snap2 := createSnapOrFatal(t, clnt, snap2Name, defProj, clone.UUID, descr2, doBlock)

	if snap2.UUID == snap.UUID {
		t.Fatalf("BUG: original snapshot id too similar to gen 2 snapshot:\n%+v\n%+v",
			snap, snap2)
	}
	getSnapEqualsOrFatal(t, clnt, byName(snapName), snap,
		fmt.Sprintf("after creating gen 2 snapshot '%s', original snapshot", snap2Name))
	getSnapEqualsOrFatal(t, clnt, byUUID(snap.UUID), snap,
		fmt.Sprintf("after creating gen 2 snapshot '%s', original snapshot", snap2Name))

	clone2Name := mkVolName()
	clone2 := createVolOrFatal(t, clnt, clone2Name, volCap, numReplicas, doCompress, nil,
		defProj, &snap2.UUID, doBlock)

	if clone2.Name != clone2Name || clone2.UUID == clone.UUID {
		t.Fatalf("BUG: clone is TOO similar to gen 2 clone:\n%+v\n%+v", clone, clone2)
	}
	if clone2.SnapshotUUID != snap2.UUID {
		t.Fatalf("BUG: gen 2 clone is not based on clone:\n%+v", clone2)
	}
	chkVolEquals(t, vol, clone2, "original volume", "its gen 2 clone",
		skipName|skipUUID|skipSnapUUID)
	chkVolEquals(t, clone, clone2, "clone", "its gen 2 clone", skipName|skipUUID|skipSnapUUID)

	getVolEqualsOrFatal(t, clnt, byUUID(vol.UUID), vol, skipETag,
		fmt.Sprintf("after creating gen 2 clone '%s', original volume", clone2Name))
	getVolEqualsOrFatal(t, clnt, byUUID(clone.UUID), clone, skipETag,
		fmt.Sprintf("after creating gen 2 clone '%s', clone", clone2Name))
	getSnapEqualsOrFatal(t, clnt, byName(snapName), snap,
		fmt.Sprintf("after creating gen 2 clone '%s', original snapshot", snapName))
	getSnapEqualsOrFatal(t, clnt, byUUID(snap.UUID), snap,
		fmt.Sprintf("after creating gen 2 clone '%s', original snapshot", snapName))

	delVolAndWaitForItOrFatal(t, clnt, clone2.UUID, "on deleted gen 2 clone")

	delSnapAndWaitForItOrFatal(t, clnt, snap2.UUID, "on deleted gen 2 snapshot")

	getVolEqualsOrFatal(t, clnt, byUUID(clone.UUID), clone, skipETag,
		fmt.Sprintf("after deleting gen 2 clone '%s', clone", clone2Name))
	getSnapEqualsOrFatal(t, clnt, byName(snapName), snap,
		fmt.Sprintf("after deleting gen 2 clone '%s', original snapshot", snapName))

	delVolAndWaitForItOrFatal(t, clnt, clone.UUID, "on deleted clone")

	delSnapAndWaitForItOrFatal(t, clnt, snap.UUID, "on deleted snapshot")

	getVolEqualsOrFatal(t, clnt, byUUID(vol.UUID), vol, skipETag,
		fmt.Sprintf("after deleting clone '%s', otiginal volume", clone2Name))

	delVolAndWaitForItOrFatal(t, clnt, vol.UUID, "on deleted volume")
}
