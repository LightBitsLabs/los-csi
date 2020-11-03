// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

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
	"github.com/lightbitslabs/lb-csi/pkg/lb"
	"github.com/lightbitslabs/lb-csi/pkg/lb/lbgrpc"
	"github.com/lightbitslabs/lb-csi/pkg/util/endpoint"
	"github.com/lightbitslabs/lb-csi/pkg/util/strlist"
	"github.com/lightbitslabs/lb-csi/pkg/util/wait"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	defaultTimeout = 10 * time.Second
	GiB            = 1024 * 1024 * 1024

	doBlock   = true
	dontBlock = false

	exists      = true
	doesntExist = false

	doAdd = true
	doDel = false

	failCap = math.MaxInt64
)

var (
	bogusUUID    = guuid.MustParse("deadbeef-e54c-4216-9456-3068e19e0b26")
	allowNoneACL = []string{lb.ACLAllowNone}
	defACL       = allowNoneACL
	prng         = rand.New(rand.NewSource(time.Now().UnixNano()))

	// flags:
	addrs           string // flag only, use `targets` in the code instead!
	clusterInfoPath string // path to JSON including cluster info
	doLog           bool

	log     = logrus.New()
	cluster *clusterInfo   // if nil - no cluster info JSON specified
	targets endpoint.Slice // filled in from `addrs` or `cluster`
)

func initFlags() {
	flag.StringVar(&addrs, "lb-addrs", "",
		"comma-separated list of LB mgmt endpoints of the form: "+
			"<addr>:<port>[,<addr>:<port>...]")
	flag.StringVar(&clusterInfoPath, "cluster-info-path", "",
		"path to JSON file with cluster info and topology")
	flag.BoolVar(&doLog, "log", false,
		"enable logger output")

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
	if doLog {
		log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02T15:04:05.000000-07:00",
		})
		log.SetLevel(logrus.DebugLevel)
	} else {
		log.SetOutput(ioutil.Discard)
		log.SetLevel(logrus.PanicLevel)
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
		flagDie("infalid cluster info file '%s' specified: %s", path, err)
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

func getCtx() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), defaultTimeout)
	return ctx
}

func mkClient(t *testing.T) *lbgrpc.Client {
	clnt, err := lbgrpc.Dial(getCtx(), log.WithField("test", t.Name()), targets)
	if err != nil {
		t.Fatalf("BUG: Dial(%s) failed: '%s'", targets, err)
	}
	return clnt
}

func mkVolName() string {
	return fmt.Sprintf("lb-csi-ut-%08x", prng.Uint32())
}

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
			ctx, _ := context.WithTimeout(context.Background(), 3*time.Second)
			clnt, err := lbgrpc.Dial(ctx, log.WithField("test", t.Name()), tc)
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

func getVolEqualsOrFatal(
	t *testing.T, clnt *lbgrpc.Client, name *string, uuid *guuid.UUID, expVol lb.Volume, expVolSrc string,
) *lb.Volume {
	var (
		vol    *lb.Volume
		err    error
		method string
		req    string
		rep    string
	)
	if name != nil {
		if uuid != nil {
			t.Fatalf("FAIL: test error, both vol name '%s' and UUID '%s' passed to "+
				"getVolEquals()", *name, *uuid)
		}
		vol, err = clnt.GetVolumeByName(getCtx(), *name)
		method = "GetVolumeByName"
		req, rep = *name, vol.UUID.String()
	} else {
		vol, err = clnt.GetVolume(getCtx(), *uuid)
		method = "GetVolume"
		req, rep = uuid.String(), vol.Name
	}
	if err != nil {
		t.Fatalf("BUG: %s(%s) failed with: '%s'", method, req, err)
	} else {
		t.Logf("OK: %s(%s) got volume: %s", method, req, rep)
	}

	if !reflect.DeepEqual(expVol, *vol) {
		t.Errorf("BUG: %s and %s(%s) results differ:\n%+v\n%+v",
			expVolSrc, method, req, expVol, *vol)
	} else {
		t.Logf("OK: %s and %s(%s) results match", expVolSrc, method, req)
	}
	return vol
}

func delVolIfExistOrFatal(
	t *testing.T, clnt *lbgrpc.Client, uuid guuid.UUID, blocking bool, existing bool,
) {
	// in some envs (e.g. InstaCluster), kicking off an async delete of an
	// empty volume can sometimes take a while:
	delCtx, _ := context.WithTimeout(context.Background(), 10*time.Second)

	err := clnt.DeleteVolume(delCtx, uuid, blocking)
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
	t *testing.T, clnt *lbgrpc.Client, uuid *guuid.UUID, vol *lb.Volume, add bool, ace string,
	expACL []string,
) (*lb.Volume, error) {
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

	newVol, err := clnt.UpdateVolume(getCtx(), *uuid, hook)
	if err != nil {
		return nil, err
	}
	if vol != nil && updateMade {
		if reflect.DeepEqual(vol, newVol) {
			t.Errorf("BUG: UpdateVolume(%s) failed to change the volume", *uuid)
		}
		if vol.ETag == newVol.ETag {
			t.Errorf("BUG: UpdateVolume(%s) failed to change the volume ETag", *uuid)
		}
	}

	if expACL == nil {
		return newVol, nil
	}

	opStr := aclOpToStr(add)
	if !strlist.AreEqual(expACL, newVol.ACL) {
		t.Errorf("BUG: UpdateVolume(%s) failed to %s ACE '%s':\nEXP: %#q\nGOT: %#q",
			*uuid, opStr, ace, expACL, newVol.ACL)
	} else {
		t.Logf("OK: UpdateVolume(%s) managed to %s ACE '%s': %#q",
			*uuid, opStr, ace, newVol.ACL)
	}

	return newVol, nil
}

func modVolACLOrFatal(
	t *testing.T, clnt *lbgrpc.Client, vol *lb.Volume, add bool, ace string, expACL []string,
) *lb.Volume {
	newVol, err := modVolACL(t, clnt, nil, vol, add, ace, expACL)
	if err != nil {
		t.Fatalf("BUG: UpdateVolume(%s) to %s ACE '%s' from ACL %#q failed with: '%s'",
			vol.UUID, aclOpToStr(add), ace, vol.ACL, err)
	}
	return newVol
}

func TestVolume(t *testing.T) {
	clnt := mkClient(t)
	defer clnt.Close()

	var volCap uint64 = 4 * 1024 * 1024 * 1024 - 4096
	var numReplicas uint32 = 3
	volName := mkVolName()
	bogusName := fmt.Sprintf("lb-csi-ut-bogus-%08x", prng.Uint32())

	vol, err := clnt.CreateVolume(getCtx(), volName, volCap, numReplicas, false, nil, doBlock)
	if err != nil {
		t.Fatalf("BUG: CreateVolume(%s) failed with: '%s'", volName, err)
	} else {
		t.Logf("OK: CreateVolume(%s) created volume UUID: %s", volName, vol.UUID)
	}
	if !strlist.AreEqual(defACL, vol.ACL) {
		t.Errorf("BUG: CreateVolume(%s) messed up ACL:\nEXP: %#q\nGOT: %#q",
		volName, defACL, vol.ACL)
	} else {
		t.Logf("OK: CreateVolume(%s) set default ACL workaround correctly", volName)
	}
	if (volCap + GiB - 1) / GiB * GiB != vol.Capacity {
		t.Errorf("BUG: CreateVolume(%s) unexpected capacity: %d for %d request",
			volName, vol.Capacity, volCap)
	} else {
		t.Logf("OK: CreateVolume(%s) set capacity within expected bounds", volName)
	}

	volUUID := vol.UUID

	bogusVol, err := clnt.GetVolumeByName(getCtx(), bogusName)
	if err == nil {
		t.Errorf("BUG: GetVolumeByName(%s) succeeded despite bogus volume name:\n%+v",
			bogusName, bogusVol)
	} else if isStatusNotFound(err) {
		t.Logf("OK: GetVolumeByName(%s) failed with 'NotFound'", bogusName)
	} else {
		t.Errorf("BUG: GetVolumeByName(%s) failed with: '%s'", bogusName, err)
	}

	vol2 := getVolEqualsOrFatal(t, clnt, &volName, nil, *vol,
		fmt.Sprintf("CreateVolume(%s)", volName))
	// sanity check:
	vol2.Capacity = 123456
	if reflect.DeepEqual(vol, vol2) {
		t.Errorf("BUG: volume comparison code is borked, above results are suspect")
	} else {
		t.Logf("OK: volume comparison code is not obviously broken")
	}

	bogusVol, err = clnt.GetVolume(getCtx(), bogusUUID)
	if err == nil {
		t.Errorf("BUG: GetVolume(%s) succeeded despite bogus volume UUID:\n%+v",
			bogusUUID, bogusVol)
	} else if isStatusNotFound(err) {
		t.Logf("OK: GetVolume(%s) failed with 'NotFound'", bogusUUID)
	} else {
		t.Errorf("BUG: GetVolume(%s) failed with: '%s'", bogusUUID, err)
	}

	vol2 = getVolEqualsOrFatal(t, clnt, nil, &volUUID, *vol,
		fmt.Sprintf("CreateVolume(%s)", volName))

	a1 := "ace-1"
	a2 := "ace-2"
	bogusVol, err = modVolACL(t, clnt, &bogusUUID, nil, doAdd, a1, defACL)
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
	vol.ETag = ""
	vol2.ETag = ""
	if !reflect.DeepEqual(vol, vol2) {
		t.Errorf("BUG: UpdateVolume(%s) changed more than ACL", volName)
	} else {
		t.Logf("OK: UpdateVolume(%s) changed only ACL", volName)
	}

	delVolIfExistOrFatal(t, clnt, bogusUUID, doBlock, doesntExist)

	delVolIfExistOrFatal(t, clnt, volUUID, doBlock, exists)

	bogusVol, err = clnt.GetVolumeByName(getCtx(), volName)
	if err == nil {
		t.Errorf("BUG: GetVolumeByName(%s) succeeded on deleted volume:\n%+v",
			volName, bogusVol)
	} else if isStatusNotFound(err) {
		t.Logf("OK: GetVolumeByName(%s) failed with 'NotFound'", volName)
	} else {
		t.Errorf("BUG: GetVolumeByName(%s) failed with: '%s'", volName, err)
	}

	// empty volumes shouldn't take any time to disappear after deletion:
	opts := wait.Backoff{Delay: 200 * time.Millisecond, Retries: 15}
	err = wait.WithExponentialBackoff(opts, func() (bool, error) {
		bogusVol, err = clnt.GetVolume(getCtx(), volUUID)
		if err == nil {
			switch bogusVol.State {
			case lb.VolumeDeleting:
				return false, nil
			default:
				return true, fmt.Errorf("BUG: GetVolume(%s) succeeded on "+
					"deleted volume:\n%+v", volUUID, bogusVol)
			}
		} else if isStatusNotFound(err) {
			t.Logf("OK: GetVolume(%s) failed with: '%s'", volUUID, err)
			return true, nil
		} else {
			return true, fmt.Errorf("BUG: GetVolume(%s) failed with: '%s'",
				volUUID, err)
		}
	})
	if err != nil {
		t.Error(err.Error())
	}
}

func TestVolumeAvailableAfterDelete(t *testing.T) {
	clnt := mkClient(t)
	defer clnt.Close()

	var volCap uint64 = 4 * 1024 * 1024 * 1024
	var numReplicas uint32 = 3
	volName := mkVolName()

	vol, err := clnt.CreateVolume(getCtx(), volName, volCap, numReplicas, false, nil, doBlock)
	if err != nil {
		t.Fatalf("BUG: CreateVolume(%s) failed with: '%s'", volName, err)
	} else {
		t.Logf("OK: CreateVolume(%s) created volume UUID: %s", volName, vol.UUID)
	}

	uuid := vol.UUID
	getVolEqualsOrFatal(t, clnt, nil, &uuid, *vol, fmt.Sprintf("CreateVolume(%s)", volName))
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
		vol, err = clnt.GetVolume(getCtx(), uuid)
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

	// currently, LightOS is supposed to always round up the capacity to
	// the nearest 1GiB:
	const capGran = 1 * 1024 * 1024 * 1024
	roundUp := func(c uint64) capacities {
		if c == 0 || c == math.MaxInt64 {
			return capacities{c, failCap}
		}
		return capacities{c, (c + capGran - 1) / capGran * capGran}
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
	vol, err := clnt.CreateVolume(getCtx(), volName, askCap, numReplicas, false, nil, dontBlock)
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
		getVol, err := clnt.GetVolume(getCtx(), vol.UUID)
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
