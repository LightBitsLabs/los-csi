// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	guuid "github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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
