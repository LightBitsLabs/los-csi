// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	guuid "github.com/google/uuid"
	"github.com/lightbitslabs/los-csi/pkg/lb"
	"github.com/lightbitslabs/los-csi/pkg/util/endpoint"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	defaultCfgDirPath         = "/etc/lb-csi"
	defaultJWTFileName        = "jwt"
	defaultBackendCfgFileName = "backend.yaml"
)

type ClientPoolMock struct {
	mock.Mock
	lb.ClientPool
}

func (m *ClientPoolMock) Close() {
	_ = m.Called()
}

func (m *ClientPoolMock) PutClient(c lb.Client) {
	_ = m.Called(c)
}

func (m *ClientPoolMock) GetClient(
	ctx context.Context, targets endpoint.Slice, mgmtScheme string,
) (lb.Client, error) {
	args := m.Called(ctx, targets, mgmtScheme)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(lb.Client), args.Error(1)
}

type ClientMock struct {
	mock.Mock
	lb.Client
}

func (m *ClientMock) Close() {
	_ = m.Called()
}

func (m *ClientMock) ID() string {
	args := m.Called()
	return args.Get(0).(string)
}

func (m *ClientMock) Targets() string {
	args := m.Called()
	return args.Get(0).(string)
}

func (m *ClientMock) RemoteOk(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *ClientMock) GetCluster(ctx context.Context) (*lb.Cluster, error) {
	args := m.Called(ctx)
	return args.Get(0).(*lb.Cluster), args.Error(1)
}

func (m *ClientMock) GetClusterInfo(ctx context.Context) (*lb.ClusterInfo, error) {
	args := m.Called(ctx)
	return args.Get(0).(*lb.ClusterInfo), args.Error(1)
}

func (m *ClientMock) ListNodes(ctx context.Context) ([]*lb.Node, error) {
	args := m.Called(ctx)
	return args.Get(0).([]*lb.Node), args.Error(1)
}

func (m *ClientMock) CreateVolume(ctx context.Context, name string, capacity uint64,
	replicaCount uint32, compress bool, acl []string, projectName string,
	snapshotID guuid.UUID, qosPolicyName string, blocking bool,
) (*lb.Volume, error) {
	args := m.Called(ctx, name, capacity,
		replicaCount, compress, acl, projectName,
		snapshotID, qosPolicyName, blocking)
	return args.Get(0).(*lb.Volume), args.Error(1)
}

func (m *ClientMock) DeleteVolume(ctx context.Context, uuid guuid.UUID, projectName string, blocking bool) error {
	args := m.Called(ctx, uuid, projectName, blocking)
	return args.Error(0)
}

func (m *ClientMock) GetVolume(ctx context.Context, uuid guuid.UUID, projectName string) (*lb.Volume, error) {
	args := m.Called(ctx, uuid, projectName)
	return args.Get(0).(*lb.Volume), args.Error(1)
}

func (m *ClientMock) GetVolumeByName(ctx context.Context, name string, projectName string) (*lb.Volume, error) {
	args := m.Called(ctx, name, projectName)
	return args.Get(0).(*lb.Volume), args.Error(1)
}

func (m *ClientMock) UpdateVolume(
	ctx context.Context,
	uuid guuid.UUID,
	projectName string,
	hook lb.VolumeUpdateHook,
) (*lb.Volume, error) {
	args := m.Called(ctx, uuid, projectName, hook)
	return args.Get(0).(*lb.Volume), args.Error(1)
}

func (m *ClientMock) CreateSnapshot(ctx context.Context, name string, projectName string, srcVolUUID guuid.UUID,
	descr string, blocking bool,
) (*lb.Snapshot, error) {
	args := m.Called(ctx, name, projectName, srcVolUUID, descr, blocking)
	return args.Get(0).(*lb.Snapshot), args.Error(1)
}

func (m *ClientMock) DeleteSnapshot(ctx context.Context, uuid guuid.UUID, projectName string, blocking bool) error {
	args := m.Called(ctx, uuid, projectName, blocking)
	return args.Error(0)
}

func (m *ClientMock) GetSnapshot(ctx context.Context, uuid guuid.UUID, projectName string) (*lb.Snapshot, error) {
	args := m.Called(ctx, uuid, projectName)
	return args.Get(0).(*lb.Snapshot), args.Error(1)
}

func (m *ClientMock) GetSnapshotByName(ctx context.Context, name string, projectName string) (*lb.Snapshot, error) {
	args := m.Called(ctx, name, projectName)
	return args.Get(0).(*lb.Snapshot), args.Error(1)
}

func getDriver(
	t *testing.T, nodeID string, rwx bool,
) (*Driver, Config, error) {
	var cfg = Config{
		DefaultBackend: "dsc",
		BackendCfgPath: filepath.Join(defaultCfgDirPath, defaultBackendCfgFileName),
		JWTPath:        filepath.Join(defaultCfgDirPath, defaultJWTFileName),

		NodeID:   nodeID,
		Endpoint: "unix:///tmp/csi.sock",

		DefaultFS: Ext4FS,

		LogLevel:      "info",
		LogRole:       "node",
		LogTimestamps: false,
		LogFormat:     "json",

		// hidden, dev-only options:
		BinaryName:    "lb-csi-plugin",
		Transport:     "tcp",
		SquelchPanics: false,
		PrettyJSON:    false,
		RWX:           rwx,
	}

	d, err := New(cfg)
	require.NoErrorf(t, err, "must not fail creating driver")
	return d, cfg, nil
}

var (
	poolOpts = lb.ClientPoolOptions{
		DialTimeout: 550 * time.Millisecond,
		LingerTime:  500 * time.Microsecond,
		ReapCycle:   100 * time.Microsecond,
	}
)

func basicClientMock(ep string) *ClientMock {
	clientMock := &ClientMock{}
	clientMock.On("ID").Return("123")
	clientMock.On("Close").Return()
	clientMock.On("Targets").Return(ep)
	clientMock.On("RemoteOk", context.Background()).Return(nil)
	return clientMock
}

func basicVolume(name string, nguid guuid.UUID, acl []string) *lb.Volume {
	vol := &lb.Volume{
		Name:         name,
		UUID:         nguid,
		ReplicaCount: 3,
		Capacity:     2,
		Compression:  false,
		ProjectName:  "default",
		ACL:          acl,
	}
	return vol
}

func TestControllerPublishVolume(t *testing.T) {
	nodeID1 := "rack01-server01"
	nodeID2 := "rack01-server02"
	ace1 := nodeIDToHostNQN(nodeID1)
	ace2 := nodeIDToHostNQN(nodeID2)
	ep := "10.19.151.24:443,10.19.151.6:443"
	projectName := "default"
	nguid := guuid.MustParse("6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66")

	testCases := []struct {
		name       string
		driver     func() (*Driver, Config)
		req        *csi.ControllerPublishVolumeRequest
		clientMock func() *ClientMock
		err        error
	}{
		{
			name: "'RWX==true' and 'AccessType==Filesystem' must fail with unsupported mode: 'MULTI_NODE_MULTI_WRITER'",
			driver: func() (*Driver, Config) {
				d, cfg, _ := getDriver(t, nodeID1, true)
				return d, cfg
			},
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: fmt.Sprintf("mgmt:%s|nguid:%s|proj:%s|scheme:grpcs", ep, nguid.String(), projectName),
				NodeId:   nodeID1,
				VolumeCapability: &csi.VolumeCapability{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: "ext4",
						},
					},
				},
			},
			clientMock: func() *ClientMock {
				clientMock := basicClientMock(ep)
				vol := basicVolume("vol1", nguid, []string{ace1})

				clientMock.On("UpdateVolume", context.Background(),
					nguid, projectName,
					mock.AnythingOfType("lb.VolumeUpdateHook")).
					Return(vol,
						status.Errorf(codes.InvalidArgument,
							"bad value of 'volume_capability.access_mode': unsupported mode: 'MULTI_NODE_MULTI_WRITER'")).
					Once()

				return clientMock
			},
			err: nil,
		},
		{
			name: "'RWX==true' and 'AccessType==Block' verify update from ALLOW_NONE to one node to two nodes",
			driver: func() (*Driver, Config) {
				d, cfg, _ := getDriver(t, nodeID1, true)
				return d, cfg
			},
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: fmt.Sprintf("mgmt:%s|nguid:%s|proj:%s|scheme:grpcs", ep, nguid.String(), projectName),
				NodeId:   nodeID1,
				VolumeCapability: &csi.VolumeCapability{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
					AccessType: &csi.VolumeCapability_Block{},
				},
			},
			clientMock: func() *ClientMock {
				clientMock := basicClientMock(ep)
				vol := basicVolume("v1", nguid, []string{lb.ACLAllowNone})
				clientMock.On("UpdateVolume", context.Background(),
					vol.UUID, vol.ProjectName,
					mock.AnythingOfType("lb.VolumeUpdateHook")).
					Return(vol,
						status.Errorf(codes.Unavailable, fmt.Sprintf("failed to publish volume to node '%s'", nodeID1))).
					Once()

				vol1 := basicVolume("v1", nguid, []string{ace1})
				clientMock.On("UpdateVolume", context.Background(),
					vol.UUID, vol.ProjectName,
					mock.AnythingOfType("lb.VolumeUpdateHook")).
					Return(vol1, nil).Once()

				vol2 := basicVolume("v1", nguid, []string{ace1, ace2})
				clientMock.On("UpdateVolume", context.Background(),
					vol.UUID, vol.ProjectName,
					mock.AnythingOfType("lb.VolumeUpdateHook")).
					Return(vol2, nil).Once()
				return clientMock
			},
			err: nil,
		},
		{
			name: "'RWX==false' must fail when asking for MULTI_NODE_MULTI_WRITER volume access mode",
			driver: func() (*Driver, Config) {
				d, cfg, _ := getDriver(t, nodeID1, false)
				return d, cfg
			},
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: fmt.Sprintf("mgmt:%s|nguid:%s|proj:%s|scheme:grpcs", ep, nguid.String(), projectName),
				NodeId:   nodeID1,
				VolumeCapability: &csi.VolumeCapability{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
					AccessType: &csi.VolumeCapability_Block{},
				},
			},
			clientMock: func() *ClientMock {
				clientMock := basicClientMock(ep)
				clientMock.On("UpdateVolume", context.Background(),
					mock.AnythingOfType("UUID"),
					projectName,
					mock.AnythingOfType("lb.VolumeUpdateHook")).
					Return(nil,
						status.Errorf(codes.InvalidArgument,
							"bad value of 'volume_capability.access_mode': unsupported mode: 'MULTI_NODE_MULTI_WRITER'")).
					Once()
				return clientMock
			},
			err: nil,
		},
		{
			name: "'RWX==false' and ask for len(ACL) > 1",
			driver: func() (*Driver, Config) {
				d, cfg, _ := getDriver(t, nodeID1, false)
				return d, cfg
			},
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: fmt.Sprintf("mgmt:%s|nguid:%s|proj:%s|scheme:grpcs", ep, nguid.String(), projectName),
				NodeId:   nodeID1,
				VolumeCapability: &csi.VolumeCapability{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
					AccessType: &csi.VolumeCapability_Block{},
				},
			},
			clientMock: func() *ClientMock {
				clientMock := basicClientMock(ep)
				vol := basicVolume("v1", nguid, []string{ace1, ace2})

				clientMock.On("UpdateVolume", context.Background(),
					vol.UUID, vol.ProjectName,
					mock.AnythingOfType("lb.VolumeUpdateHook")).
					Return(vol,
						status.Errorf(codes.Unavailable, fmt.Sprintf("failed to publish volume to node '%s'", nodeID1))).Once()
				return clientMock
			},
			err: nil,
		},
		{
			name: "'RWX==false' and ask RWO vol cap",
			driver: func() (*Driver, Config) {
				d, cfg, _ := getDriver(t, nodeID1, false)
				return d, cfg
			},
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: fmt.Sprintf("mgmt:%s|nguid:%s|proj:%s|scheme:grpcs", ep, nguid.String(), projectName),
				NodeId:   nodeID1,
				VolumeCapability: &csi.VolumeCapability{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: "ext4",
						},
					},
				},
			},
			clientMock: func() *ClientMock {
				clientMock := basicClientMock(ep)

				vol := basicVolume("v1", nguid, []string{ace1})

				clientMock.On("UpdateVolume", context.Background(),
					vol.UUID, vol.ProjectName,
					mock.AnythingOfType("lb.VolumeUpdateHook")).
					Return(vol, nil).Once()
				return clientMock
			},
			err: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientMock := tc.clientMock()
			driver, _ := tc.driver()
			driver.lbclients = lb.NewClientPoolWithOptions(
				func(ctx context.Context, targets endpoint.Slice, mgmtScheme string) (lb.Client, error) {
					return clientMock, nil
				},
				poolOpts,
			)
			for _, call := range clientMock.ExpectedCalls {
				if call.Method == "UpdateVolume" {
					updateVolumeCall := call
					callErr := updateVolumeCall.ReturnArguments.Error(1)
					_, err := driver.ControllerPublishVolume(context.Background(), tc.req)
					if callErr != nil {
						require.EqualError(t, err, callErr.Error())
					} else {
						require.NoErrorf(t, err, "should not fail publishing volume")
					}
				}
			}
		})
	}
}

func TestControllerUnpublishVolume(t *testing.T) {
	nodeID1 := "rack01-server01"
	nodeID2 := "rack01-server02"
	ace1 := nodeIDToHostNQN(nodeID1)
	ace2 := nodeIDToHostNQN(nodeID2)
	ep := "10.19.151.24:443,10.19.151.6:443"
	projectName := "default"
	nguid := guuid.MustParse("6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66")

	testCases := []struct {
		name       string
		driver     func() (*Driver, Config)
		req        *csi.ControllerUnpublishVolumeRequest
		clientMock func() *ClientMock
		err        error
	}{
		{
			name: "'RWX==true' vol published to single node should be updated to ALLOW_NONE",
			driver: func() (*Driver, Config) {
				d, cfg, _ := getDriver(t, nodeID1, true)
				return d, cfg
			},
			req: &csi.ControllerUnpublishVolumeRequest{
				VolumeId: fmt.Sprintf("mgmt:%s|nguid:%s|proj:%s|scheme:grpcs", ep, nguid.String(), projectName),
				NodeId:   nodeID1,
			},
			clientMock: func() *ClientMock {
				clientMock := basicClientMock(ep)
				vol := basicVolume("v1", nguid, []string{ace1})

				clientMock.On("UpdateVolume", context.Background(),
					vol.UUID, vol.ProjectName,
					mock.AnythingOfType("lb.VolumeUpdateHook")).
					Return(vol, mkEagain("failed to unpublish volume from node '%s'", nodeID1)).Once()

				vol1 := basicVolume("v1", nguid, []string{lb.ACLAllowNone})
				clientMock.On("UpdateVolume", context.Background(),
					vol.UUID, vol.ProjectName,
					mock.AnythingOfType("lb.VolumeUpdateHook")).
					Return(vol1, nil).Once()
				return clientMock
			},
			err: nil,
		},
		{
			name: "'RWX==true' vol published to single node should be updated to ALLOW_NONE",
			driver: func() (*Driver, Config) {
				d, cfg, _ := getDriver(t, nodeID1, true)
				return d, cfg
			},
			req: &csi.ControllerUnpublishVolumeRequest{
				VolumeId: fmt.Sprintf("mgmt:%s|nguid:%s|proj:%s|scheme:grpcs", ep, nguid.String(), projectName),
				NodeId:   nodeID1,
			},
			clientMock: func() *ClientMock {
				clientMock := basicClientMock(ep)
				vol := basicVolume("v1", nguid, []string{ace1, ace2})

				clientMock.On("UpdateVolume", context.Background(),
					vol.UUID, vol.ProjectName,
					mock.AnythingOfType("lb.VolumeUpdateHook")).
					Return(vol, mkEagain("failed to unpublish volume from node '%s'", nodeID1)).Once()

				vol1 := basicVolume("v1", nguid, []string{ace2})
				clientMock.On("UpdateVolume", context.Background(),
					vol.UUID, vol.ProjectName,
					mock.AnythingOfType("lb.VolumeUpdateHook")).
					Return(vol1, nil).Once()
				return clientMock
			},
			err: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clientMock := tc.clientMock()
			driver, _ := tc.driver()
			driver.lbclients = lb.NewClientPoolWithOptions(
				func(ctx context.Context, targets endpoint.Slice, mgmtScheme string) (lb.Client, error) {
					return clientMock, nil
				},
				poolOpts,
			)
			for _, call := range clientMock.ExpectedCalls {
				if call.Method == "UpdateVolume" {
					updateVolumeCall := call
					callErr := updateVolumeCall.ReturnArguments.Error(1)
					_, err := driver.ControllerUnpublishVolume(context.Background(), tc.req)
					if callErr != nil {
						require.EqualError(t, err, callErr.Error())
					} else {
						require.NoErrorf(t, err, "should not fail publishing volume")
					}
				}
			}
		})
	}
}
