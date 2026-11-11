// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"testing"

	guuid "github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/lightbitslabs/los-csi/pkg/lb"
	"github.com/lightbitslabs/los-csi/pkg/util/endpoint"
	"github.com/lightbitslabs/los-csi/pkg/util/strlist"
)

// the route lookup toward a loopback target always picks this source.
const loopbackAddr = "127.0.0.1"

func mkRecheckVid(t *testing.T, mgmtEP string) lbResourceID {
	eps, err := endpoint.ParseCSV(mgmtEP)
	require.NoError(t, err)
	return lbResourceID{mgmtEPs: eps, uuid: guuid.New(), projName: "proj", scheme: "grpcs", ipACL: true}
}

func mkRecheckDriver(clnt *ClientMock) *Driver {
	d := &Driver{log: logrus.NewEntry(logrus.New())}
	d.lbclients = lb.NewClientPoolWithOptions(
		func(context.Context, endpoint.Slice, string) (lb.Client, error) { return clnt, nil },
		poolOpts,
	)
	return d
}

func mkRecheckClient(ep string) *ClientMock {
	clnt := &ClientMock{}
	clnt.On("ID").Return("recheck").Maybe()
	clnt.On("Targets").Return(ep).Maybe()
	clnt.On("Close").Return().Maybe()
	clnt.On("RemoteOk", mock.Anything).Return(nil).Maybe()
	clnt.On("GetClusterInfo", mock.Anything).
		Return(&lb.ClusterInfo{NvmeEndpoints: []string{loopbackAddr + ":4420"}}, nil).Once()
	return clnt
}

// onUpdateVolume runs the update hook against a volume with IP-ACL `cur`,
// and stores the update it asks for in `got`.
func onUpdateVolume(clnt *ClientMock, vid lbResourceID, cur []string, err error, got **lb.VolumeUpdate) {
	clnt.On("UpdateVolume", mock.Anything, vid.uuid, vid.projName, mock.Anything).
		Run(func(args mock.Arguments) {
			if err != nil {
				return
			}
			upd, herr := args.Get(3).(lb.VolumeUpdateHook)(&lb.Volume{IPAcl: cur})
			if herr == nil {
				*got = upd
			}
		}).
		Return(&lb.Volume{}, err).Once()
}

func TestIPAclVolListTrack(t *testing.T) {
	var l ipACLVolList
	vid := mkRecheckVid(t, "10.0.0.1:443")

	l.track(vid, "jwt1", []string{"10.0.0.5"})
	l.track(vid, "jwt2", nil)
	vols := l.list()
	require.Len(t, vols, 1)
	require.Equal(t, "jwt2", vols[0].jwt)
	require.Equal(t, []string{"10.0.0.5"}, vols[0].addrs, "a retry must keep the enrolled addresses")

	stale := vols[0]
	l.track(vid, "jwt3", []string{"10.0.0.6"})
	l.setAddrs(stale, []string{"10.0.0.7"})
	l.drop(stale)
	vols = l.list()
	require.Len(t, vols, 1, "a stale re-check result must not drop a newer entry")
	require.Equal(t, []string{"10.0.0.6"}, vols[0].addrs)

	l.untrack(vid.uuid)
	require.Empty(t, l.list())
}

func TestRecheckIPAcl(t *testing.T) {
	testCases := []struct {
		name      string
		enrolled  []string
		cur       []string
		updateErr error
		wantACL   []string
		wantAddrs []string // nil: the volume is no longer tracked.
	}{
		{
			name:      "unchanged addresses need no update",
			enrolled:  []string{loopbackAddr},
			wantAddrs: []string{loopbackAddr},
		},
		{
			name:      "new address is enrolled",
			enrolled:  []string{"10.0.0.9"},
			cur:       []string{"10.0.0.9"},
			wantACL:   []string{"10.0.0.9", loopbackAddr},
			wantAddrs: []string{loopbackAddr},
		},
		{
			name:     "ALLOW_NONE after the last detach stops the re-check",
			enrolled: []string{"10.0.0.9"},
			cur:      []string{lb.ACLAllowNone},
		},
		{
			name:      "update error keeps the entry for the next re-check",
			enrolled:  []string{"10.0.0.9"},
			updateErr: status.Error(codes.Unavailable, "down"),
			wantAddrs: []string{"10.0.0.9"},
		},
		{
			name:      "deleted volume stops the re-check",
			enrolled:  []string{"10.0.0.9"},
			updateErr: status.Error(codes.NotFound, "no such volume"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			vid := mkRecheckVid(t, "127.0.0.1:443")
			clnt := mkRecheckClient("127.0.0.1:443")
			var got *lb.VolumeUpdate
			if !strlist.AreEqual(tc.enrolled, []string{loopbackAddr}) {
				onUpdateVolume(clnt, vid, tc.cur, tc.updateErr, &got)
			}
			d := mkRecheckDriver(clnt)
			d.ipACLVols.track(vid, "jwt", tc.enrolled)

			d.recheckIPAcl(context.Background())

			clnt.AssertExpectations(t)
			if tc.wantACL != nil {
				require.NotNil(t, got)
				require.Equal(t, tc.wantACL, got.IPAcl)
			} else {
				require.Nil(t, got, "no IP-ACL update expected")
			}
			vols := d.ipACLVols.list()
			if tc.wantAddrs == nil {
				require.Empty(t, vols)
				return
			}
			require.Len(t, vols, 1)
			require.Equal(t, tc.wantAddrs, vols[0].addrs)
		})
	}
}

func TestRecheckIPAclOneClusterInfoPerCluster(t *testing.T) {
	clnt := mkRecheckClient("127.0.0.1:443")
	d := mkRecheckDriver(clnt)
	var got1, got2 *lb.VolumeUpdate
	vid1 := mkRecheckVid(t, "127.0.0.1:443")
	vid2 := mkRecheckVid(t, "127.0.0.1:443")
	onUpdateVolume(clnt, vid1, []string{"10.0.0.9"}, nil, &got1)
	onUpdateVolume(clnt, vid2, []string{"10.0.0.8"}, nil, &got2)
	d.ipACLVols.track(vid1, "jwt", []string{"10.0.0.9"})
	d.ipACLVols.track(vid2, "jwt", []string{"10.0.0.8"})

	d.recheckIPAcl(context.Background())

	clnt.AssertExpectations(t)
	require.Equal(t, []string{"10.0.0.9", loopbackAddr}, got1.IPAcl)
	require.Equal(t, []string{"10.0.0.8", loopbackAddr}, got2.IPAcl)
}

func ctxHasJWT(jwt string) interface{} {
	return mock.MatchedBy(func(ctx context.Context) bool {
		md, _ := metadata.FromOutgoingContext(ctx)
		return strlist.Contains(md.Get("authorization"), "Bearer "+jwt)
	})
}

func TestRecheckClusterInfoTriesEachJWT(t *testing.T) {
	testCases := []struct {
		name      string
		firstErr  error
		wantFound bool
	}{
		{
			name:      "rejected JWT falls back to the next volume's JWT",
			firstErr:  status.Error(codes.Unauthenticated, "token expired"),
			wantFound: true,
		},
		{
			name:     "unreachable cluster is not retried with other JWTs",
			firstErr: status.Error(codes.Unavailable, "down"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			clnt := &ClientMock{}
			clnt.On("ID").Return("recheck").Maybe()
			clnt.On("Targets").Return("127.0.0.1:443").Maybe()
			clnt.On("Close").Return().Maybe()
			clnt.On("RemoteOk", mock.Anything).Return(nil).Maybe()
			clnt.On("GetClusterInfo", ctxHasJWT("first")).Return((*lb.ClusterInfo)(nil), tc.firstErr).Once()
			if tc.wantFound {
				clnt.On("GetClusterInfo", ctxHasJWT("second")).
					Return(&lb.ClusterInfo{NvmeEndpoints: []string{loopbackAddr + ":4420"}}, nil).Once()
			}
			d := mkRecheckDriver(clnt)
			vols := []*ipACLVol{
				{vid: mkRecheckVid(t, "127.0.0.1:443"), jwt: "first"},
				{vid: mkRecheckVid(t, "127.0.0.1:443"), jwt: "first"},
				{vid: mkRecheckVid(t, "127.0.0.1:443"), jwt: "second"},
			}

			got, ci, err := d.recheckClusterInfo(context.Background(), vols)

			clnt.AssertExpectations(t)
			if !tc.wantFound {
				require.Equal(t, codes.Unavailable, status.Code(err))
				return
			}
			require.NoError(t, err)
			require.NotNil(t, ci)
			d.PutLBClient(got)
		})
	}
}
