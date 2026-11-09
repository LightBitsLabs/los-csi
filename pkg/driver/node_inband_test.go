// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	guuid "github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	mountutils "k8s.io/mount-utils"

	"github.com/lightbitslabs/los-csi/pkg/driver/backend"
	"github.com/lightbitslabs/los-csi/pkg/lb"
	"github.com/lightbitslabs/los-csi/pkg/util/endpoint"
)

var (
	fullPair    = lb.TrustedHostSecrets{HostSecret: "DHHC-1:00:aaa:", TargetSecret: "DHHC-1:00:bbb:"}
	otherPair   = lb.TrustedHostSecrets{HostSecret: "DHHC-1:00:ccc:", TargetSecret: "DHHC-1:00:ddd:"}
	emptyPair   = lb.TrustedHostSecrets{}
	authCluster = &lb.ClusterInfo{UUID: guuid.New(), InBandAuthEnabled: true}
)

func TestDCAuthConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "discovery-client.yaml")

	got, err := readDCAuthConfig(path)
	require.NoError(t, err, "missing file must not be an error")
	require.Equal(t, emptyPair, got)

	require.NoError(t, writeDCAuthConfig(path, fullPair))
	got, err = readDCAuthConfig(path)
	require.NoError(t, err)
	require.Equal(t, fullPair, got)

	require.NoError(t, writeDCAuthConfig(path, otherPair))
	got, err = readDCAuthConfig(path)
	require.NoError(t, err)
	require.Equal(t, otherPair, got)
}

func TestDCAuthConfigPreservesForeignKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "discovery-client.yaml")
	foreign := "maxIOQueues: 4\nclientConfigDir: /etc/discovery-client/discovery.d\n"
	require.NoError(t, os.WriteFile(path, []byte(foreign+"dhChapSecret: \"stale\"\n"), 0o600))

	require.NoError(t, writeDCAuthConfig(path, fullPair))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(raw), "maxIOQueues: 4")
	require.Contains(t, string(raw), "clientConfigDir: /etc/discovery-client/discovery.d")
	require.NotContains(t, string(raw), "stale")
	got, err := readDCAuthConfig(path)
	require.NoError(t, err)
	require.Equal(t, fullPair, got)
}

func mkInBandDriver(t *testing.T, dcConfigPath string) *Driver {
	return &Driver{
		nodeID:       "test-node",
		hostNQN:      nodeIDToHostNQN("test-node"),
		dcConfigPath: dcConfigPath,
		inBandAuth:   dcConfigPath != "",
		log:          logrus.NewEntry(logrus.New()),
	}
}

func mkInBandVid(t *testing.T) lbResourceID {
	eps, err := endpoint.ParseCSV("10.0.0.1:443")
	require.NoError(t, err)
	return lbResourceID{
		mgmtEPs:  eps,
		uuid:     guuid.New(),
		projName: "proj",
		scheme:   "grpcs",
	}
}

func TestEnsureInBandAuthDisabledCluster(t *testing.T) {
	d := mkInBandDriver(t, "")
	clnt := &ClientMock{}
	ci := &lb.ClusterInfo{UUID: guuid.New(), InBandAuthEnabled: false}
	err := d.ensureInBandAuth(context.Background(), d.log, clnt, ci, mkInBandVid(t))
	require.NoError(t, err, "disabled cluster must be a no-op")
	clnt.AssertExpectations(t)
}

func TestEnsureInBandAuthDisabledClusterInBandPlugin(t *testing.T) {
	d := mkInBandDriver(t, filepath.Join(t.TempDir(), "discovery-client.yaml"))
	clnt := &ClientMock{}
	ci := &lb.ClusterInfo{UUID: guuid.New(), InBandAuthEnabled: false}
	err := d.ensureInBandAuth(context.Background(), d.log, clnt, ci, mkInBandVid(t))
	require.Equal(t, codes.FailedPrecondition, status.Code(err),
		"in-band auth plugin on a disabled cluster must refuse the stage")
	clnt.AssertExpectations(t)
}

func TestEnsureInBandAuthNoConfigPath(t *testing.T) {
	d := mkInBandDriver(t, "")
	err := d.ensureInBandAuth(context.Background(), d.log, &ClientMock{}, authCluster, mkInBandVid(t))
	require.Error(t, err, "enabled cluster without a DC config path must fail")
}

func TestEnsureInBandAuthFreshNode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "discovery-client.yaml")
	d := mkInBandDriver(t, path)
	vid := mkInBandVid(t)

	clnt := &ClientMock{}
	notFound := status.Error(codes.NotFound, "no such trusted host")
	clnt.On("GetTrustedHost", mock.Anything, "test-node", "proj").Return((*lb.TrustedHost)(nil), notFound).Once()
	clnt.On("CreateTrustedHost", mock.Anything, "test-node", "proj", d.hostNQN).
		Return(&lb.TrustedHost{Name: "test-node"}, nil).Once()
	clnt.On("GetTrustedHostSecrets", mock.Anything, "test-node", "proj").
		Return(&emptyPair, nil).Once()
	clnt.On("SetTrustedHostSecrets", mock.Anything, "test-node", "proj", emptyPair).
		Return(nil).Once()
	clnt.On("GetTrustedHostSecrets", mock.Anything, "test-node", "proj").
		Return(&fullPair, nil).Once()

	err := d.ensureInBandAuth(context.Background(), d.log, clnt, authCluster, vid)
	require.NoError(t, err)
	clnt.AssertExpectations(t)

	got, err := readDCAuthConfig(path)
	require.NoError(t, err)
	require.Equal(t, fullPair, got, "auto-generated pair must be persisted for the DC")
}

func TestEnsureInBandAuthAlreadyRegistered(t *testing.T) {
	path := filepath.Join(t.TempDir(), "discovery-client.yaml")
	require.NoError(t, writeDCAuthConfig(path, fullPair))
	d := mkInBandDriver(t, path)

	clnt := &ClientMock{}
	clnt.On("GetTrustedHost", mock.Anything, "test-node", "proj").
		Return(&lb.TrustedHost{Name: "test-node"}, nil).Once()
	clnt.On("GetTrustedHostSecrets", mock.Anything, "test-node", "proj").
		Return(&fullPair, nil).Once()

	err := d.ensureInBandAuth(context.Background(), d.log, clnt, authCluster, mkInBandVid(t))
	require.NoError(t, err, "matching pair must be a no-op")
	clnt.AssertExpectations(t)
}

func TestEnsureInBandAuthPushesNodePairToNewCluster(t *testing.T) {
	path := filepath.Join(t.TempDir(), "discovery-client.yaml")
	require.NoError(t, writeDCAuthConfig(path, fullPair))
	d := mkInBandDriver(t, path)

	clnt := &ClientMock{}
	clnt.On("GetTrustedHost", mock.Anything, "test-node", "proj").
		Return(&lb.TrustedHost{Name: "test-node"}, nil).Once()
	clnt.On("GetTrustedHostSecrets", mock.Anything, "test-node", "proj").
		Return(&emptyPair, nil).Once()
	clnt.On("SetTrustedHostSecrets", mock.Anything, "test-node", "proj", fullPair).
		Return(nil).Once()

	err := d.ensureInBandAuth(context.Background(), d.log, clnt, authCluster, mkInBandVid(t))
	require.NoError(t, err, "existing node pair must be pushed to the new cluster")
	clnt.AssertExpectations(t)
}

func TestEnsureInBandAuthAdoptsClusterPair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "discovery-client.yaml")
	d := mkInBandDriver(t, path)

	clnt := &ClientMock{}
	clnt.On("GetTrustedHost", mock.Anything, "test-node", "proj").
		Return(&lb.TrustedHost{Name: "test-node"}, nil).Once()
	clnt.On("GetTrustedHostSecrets", mock.Anything, "test-node", "proj").
		Return(&fullPair, nil).Once()

	err := d.ensureInBandAuth(context.Background(), d.log, clnt, authCluster, mkInBandVid(t))
	require.NoError(t, err)
	clnt.AssertExpectations(t)

	got, err := readDCAuthConfig(path)
	require.NoError(t, err)
	require.Equal(t, fullPair, got, "cluster-held pair must be adopted locally")
}

// a stateful mock client standing in for the cluster: the trusted host and
// its secrets behave like the real API, including server-side generation on
// an empty Set, so concurrent callers exercise real interleavings.
type ibaClusterSim struct {
	ClientMock
	mu      sync.Mutex
	host    *lb.TrustedHost
	pair    lb.TrustedHostSecrets
	autogen int
}

func (c *ibaClusterSim) GetTrustedHost(
	ctx context.Context, name, proj string,
) (*lb.TrustedHost, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.host == nil {
		return nil, status.Error(codes.NotFound, "no such trusted host")
	}
	return c.host, nil
}

func (c *ibaClusterSim) CreateTrustedHost(
	ctx context.Context, name, proj, hostNQN string,
) (*lb.TrustedHost, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.host != nil {
		return nil, status.Error(codes.AlreadyExists, "host NQN already used")
	}
	c.host = &lb.TrustedHost{Name: name, ProjectName: proj, HostNQN: hostNQN}
	return c.host, nil
}

func (c *ibaClusterSim) GetTrustedHostSecrets(
	ctx context.Context, name, proj string,
) (*lb.TrustedHostSecrets, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.host == nil {
		return nil, status.Error(codes.NotFound, "no such trusted host")
	}
	pair := c.pair
	return &pair, nil
}

func (c *ibaClusterSim) SetTrustedHostSecrets(
	ctx context.Context, name, proj string, secrets lb.TrustedHostSecrets,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.host == nil {
		return status.Error(codes.NotFound, "no such trusted host")
	}
	if secrets.HostSecret == "" {
		c.autogen++
		secrets = lb.TrustedHostSecrets{
			HostSecret:   fmt.Sprintf("DHHC-1:00:gen%d:", c.autogen),
			TargetSecret: fmt.Sprintf("DHHC-1:00:tgt%d:", c.autogen),
		}
	}
	c.pair = secrets
	return nil
}

func TestEnsureInBandAuthConcurrentFirstStages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "discovery-client.yaml")
	d := mkInBandDriver(t, path)
	sim := &ibaClusterSim{}

	const stages = 8
	var wg sync.WaitGroup
	errs := make([]error, stages)
	for i := 0; i < stages; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = d.ensureInBandAuth(
				context.Background(), d.log, sim, authCluster, mkInBandVid(t))
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		require.NoErrorf(t, err, "stage %d failed", i)
	}
	require.Equal(t, 1, sim.autogen,
		"concurrent first stages must generate the node pair exactly once")
	got, err := readDCAuthConfig(path)
	require.NoError(t, err)
	require.Equal(t, sim.pair, got,
		"the DC config and the cluster must hold the same pair")
}

func TestDCAuthConfigKeepsBlankLinesAndNestedKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "discovery-client.yaml")
	foreign := "maxIOQueues: 4\n\nlogging:\n  dhChapSecret: \"not-ours\"\n\n"
	require.NoError(t, os.WriteFile(path, []byte(foreign+"dhChapSecret: \"stale\"\n"), 0o600))

	require.NoError(t, writeDCAuthConfig(path, fullPair))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(raw), "maxIOQueues: 4\n\nlogging:")
	require.Contains(t, string(raw), "  dhChapSecret: \"not-ours\"",
		"an indented key under a nested map must be left alone")
	require.NotContains(t, string(raw), "stale")
	require.Equal(t, fullPair, parseDCAuthConfig(raw))
}

// eligibleBackend admits every volume; any other backend call panics.
type eligibleBackend struct {
	backend.Backend
}

func (eligibleBackend) LBVolEligible(context.Context, *lb.Volume) *status.Status {
	return nil
}

func TestNodeStageInBandAuthMismatchLeavesIPAclAlone(t *testing.T) {
	testCases := []struct {
		name       string
		pluginIBA  bool
		clusterIBA bool
	}{
		{name: "plugin with in-band auth, cluster without", pluginIBA: true},
		{name: "plugin without in-band auth, cluster with", clusterIBA: true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dcConfigPath := ""
			if tc.pluginIBA {
				dcConfigPath = filepath.Join(t.TempDir(), "discovery-client.yaml")
			}
			d := mkInBandDriver(t, dcConfigPath)
			d.be = eligibleBackend{}
			d.mounter = &mountutils.SafeFormatAndMount{Interface: mountutils.NewFakeMounter(nil)}

			nguid := guuid.New()
			clnt := &ClientMock{}
			clnt.On("ID").Return("123")
			clnt.On("Close").Return()
			clnt.On("Targets").Return("10.0.0.1:443")
			clnt.On("RemoteOk", mock.Anything).Return(nil)
			clnt.On("GetClusterInfo", mock.Anything).Return(&lb.ClusterInfo{
				UUID:               guuid.New(),
				InBandAuthEnabled:  tc.clusterIBA,
				DiscoveryEndpoints: []string{"10.0.0.2:8009"},
				NvmeEndpoints:      []string{"10.0.0.2:4420"},
			}, nil)
			clnt.On("GetVolume", mock.Anything, nguid, "proj").Return(&lb.Volume{
				UUID:       nguid,
				State:      lb.VolumeAvailable,
				Protection: lb.VolumeProtected,
				ACL:        []string{d.hostNQN},
			}, nil)
			d.lbclients = lb.NewClientPoolWithOptions(
				func(ctx context.Context, targets endpoint.Slice, mgmtScheme string) (lb.Client, error) {
					return clnt, nil
				},
				poolOpts,
			)

			_, err := d.NodeStageVolume(context.Background(), &csi.NodeStageVolumeRequest{
				VolumeId:          fmt.Sprintf("mgmt:10.0.0.1:443|nguid:%s|proj:proj|scheme:grpcs|ipacl:enabled", nguid),
				StagingTargetPath: t.TempDir(),
				VolumeCapability: &csi.VolumeCapability{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{FsType: "ext4"},
					},
				},
			})
			require.Equal(t, codes.FailedPrecondition, status.Code(err), "got: %v", err)
			clnt.AssertNotCalled(t, "UpdateVolume", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}
