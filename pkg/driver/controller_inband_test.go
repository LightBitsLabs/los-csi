// Copyright (C) 2016--2026 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	guuid "github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/keymutex"

	"github.com/lightbitslabs/los-csi/pkg/lb"
	"github.com/lightbitslabs/los-csi/pkg/util/endpoint"
)

const reapNode = "worker-3.node"

type fakeAttachments struct {
	ids []string
	err error
}

func (f *fakeAttachments) AttachedVolumeIDs(context.Context, string) ([]string, error) {
	return f.ids, f.err
}

func reapVolID(eps, proj string) string {
	return fmt.Sprintf("mgmt:%s|nguid:%s|proj:%s|scheme:grpcs", eps, guuid.New(), proj)
}

func mkReapDriver(att nodeAttachments) *Driver {
	return &Driver{
		log:         logrus.NewEntry(logrus.New()),
		nodeLocks:   keymutex.NewHashed(0),
		attachments: att,
	}
}

func mkReapVid(t *testing.T, eps, proj string) lbResourceID {
	vid, err := parseCSIResourceID(reapVolID(eps, proj))
	require.NoError(t, err)
	return vid
}

func volIDs(vids ...lbResourceID) []string {
	ids := make([]string, len(vids))
	for i, vid := range vids {
		ids[i] = vid.String()
	}
	return ids
}

func TestReapTrustedHost(t *testing.T) {
	const thisCluster = "10.0.0.1:443,10.0.0.2:443"
	thNotFound := status.Error(codes.NotFound, "no such trusted host")
	volNotFound := status.Error(codes.NotFound, "no such volume")

	self := mkReapVid(t, thisCluster, "proj")
	sameCluster := mkReapVid(t, "10.0.0.3:443", "proj")
	otherCluster := mkReapVid(t, thisCluster, "proj")
	otherProject := mkReapVid(t, thisCluster, "other")

	testCases := []struct {
		name      string
		att       nodeAttachments
		ci        *lb.ClusterInfo
		lookups   map[guuid.UUID]error
		deleteErr error
		expDelete bool
	}{{
		name:      "last volume of the cluster detached",
		att:       &fakeAttachments{},
		ci:        authCluster,
		expDelete: true,
	}, {
		name:      "trusted host already gone",
		att:       &fakeAttachments{},
		ci:        authCluster,
		deleteErr: thNotFound,
		expDelete: true,
	}, {
		name:    "same cluster and project still attached through other endpoints",
		att:     &fakeAttachments{ids: volIDs(sameCluster)},
		ci:      authCluster,
		lookups: map[guuid.UUID]error{sameCluster.uuid: nil},
	}, {
		name:      "same project only on another cluster",
		att:       &fakeAttachments{ids: volIDs(otherCluster)},
		ci:        authCluster,
		lookups:   map[guuid.UUID]error{otherCluster.uuid: volNotFound},
		expDelete: true,
	}, {
		name:      "only volumes of another project attached",
		att:       &fakeAttachments{ids: volIDs(otherProject)},
		ci:        authCluster,
		expDelete: true,
	}, {
		name:      "detaching volume listed itself",
		att:       &fakeAttachments{ids: volIDs(self)},
		ci:        authCluster,
		expDelete: true,
	}, {
		name:    "attached volume lookup fails",
		att:     &fakeAttachments{ids: volIDs(sameCluster)},
		ci:      authCluster,
		lookups: map[guuid.UUID]error{sameCluster.uuid: status.Error(codes.Unavailable, "down")},
	}, {
		name: "unparsable attached volume handle",
		att:  &fakeAttachments{ids: []string{"garbage"}},
		ci:   authCluster,
	}, {
		name: "attachments unknown",
		att:  &fakeAttachments{err: errors.New("apiserver unavailable")},
		ci:   authCluster,
	}, {
		name: "cluster without in-band auth",
		att:  &fakeAttachments{},
		ci:   &lb.ClusterInfo{UUID: guuid.New()},
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d := mkReapDriver(tc.att)
			clnt := &ClientMock{}
			clnt.On("GetClusterInfo", mock.Anything).Return(tc.ci, nil).Once()
			for uuid, err := range tc.lookups {
				vol := &lb.Volume{UUID: uuid}
				if err != nil {
					vol = nil
				}
				clnt.On("GetVolume", mock.Anything, uuid, "proj").Return(vol, err).Once()
			}
			if tc.expDelete {
				clnt.On("DeleteTrustedHost", mock.Anything, reapNode, "proj").
					Return(tc.deleteErr).Once()
			}
			d.reapTrustedHost(context.Background(), clnt, d.log, self, reapNode)
			clnt.AssertExpectations(t)
		})
	}
}

// a pod with two volumes: the trusted host must stay while either volume is
// attached, and must go once both detach, also when they detach together.
func TestReapPodWithTwoVolumes(t *testing.T) {
	const ep = "10.0.0.1:443"
	v1, v2 := mkReapVid(t, ep, "proj"), mkReapVid(t, ep, "proj")

	reap := func(detaching lbResourceID, vas ...*storagev1.VolumeAttachment) *ClientMock {
		objs := []runtime.Object{mkPV("pv-1", v1.String()), mkPV("pv-2", v2.String())}
		for _, va := range vas {
			objs = append(objs, va)
		}
		d := mkReapDriver(&k8sNodeAttachments{cs: fake.NewSimpleClientset(objs...)})
		clnt := &ClientMock{}
		clnt.On("GetClusterInfo", mock.Anything).Return(authCluster, nil)
		clnt.On("GetVolume", mock.Anything, mock.Anything, "proj").Return(&lb.Volume{}, nil)
		clnt.On("DeleteTrustedHost", mock.Anything, reapNode, "proj").Return(nil)
		d.reapTrustedHost(context.Background(), clnt, d.log, detaching, reapNode)
		return clnt
	}

	t.Run("first volume detaches", func(t *testing.T) {
		clnt := reap(v1,
			mkVA("va-1", driverName, reapNode, "pv-1", true),
			mkVA("va-2", driverName, reapNode, "pv-2", false))
		clnt.AssertCalled(t, "GetVolume", mock.Anything, v2.uuid, "proj")
		clnt.AssertNotCalled(t, "DeleteTrustedHost", mock.Anything, mock.Anything, mock.Anything)
	})
	t.Run("second volume detaches", func(t *testing.T) {
		clnt := reap(v2, mkVA("va-2", driverName, reapNode, "pv-2", true))
		clnt.AssertCalled(t, "DeleteTrustedHost", mock.Anything, reapNode, "proj")
	})
	t.Run("both volumes detach together", func(t *testing.T) {
		for _, vid := range []lbResourceID{v1, v2} {
			clnt := reap(vid,
				mkVA("va-1", driverName, reapNode, "pv-1", true),
				mkVA("va-2", driverName, reapNode, "pv-2", true))
			clnt.AssertCalled(t, "DeleteTrustedHost", mock.Anything, reapNode, "proj")
		}
	})
}

func TestReapTrustedHostDisabled(t *testing.T) {
	d := mkReapDriver(nil)
	clnt := &ClientMock{}
	d.reapTrustedHost(context.Background(), clnt, d.log, mkReapVid(t, "10.0.0.1:443", "proj"), reapNode)
	clnt.AssertExpectations(t)
}

func TestControllerUnpublishVolumeReapsTrustedHost(t *testing.T) {
	const ep = "10.0.0.1:443"
	vid := mkReapVid(t, ep, "proj")

	clnt := basicClientMock(ep)
	vol := basicVolume("v1", vid.uuid, []string{lb.ACLAllowNone})
	vol.ProjectName = vid.projName
	clnt.On("UpdateVolume", mock.Anything, vid.uuid, vid.projName,
		mock.AnythingOfType("lb.VolumeUpdateHook")).Return(vol, nil).Once()
	clnt.On("GetClusterInfo", mock.Anything).Return(authCluster, nil).Once()
	clnt.On("DeleteTrustedHost", mock.Anything, reapNode, vid.projName).Return(nil).Once()

	d := mkReapDriver(&fakeAttachments{})
	d.lbclients = lb.NewClientPoolWithOptions(
		func(context.Context, endpoint.Slice, string) (lb.Client, error) {
			return clnt, nil
		},
		poolOpts,
	)
	_, err := d.ControllerUnpublishVolume(context.Background(), &csi.ControllerUnpublishVolumeRequest{
		VolumeId: vid.String(),
		NodeId:   reapNode,
	})
	require.NoError(t, err)
	clnt.AssertCalled(t, "DeleteTrustedHost", mock.Anything, reapNode, vid.projName)
}

func mkVA(name, attacher, nodeID, pv string, deleting bool) *storagev1.VolumeAttachment {
	va := &storagev1.VolumeAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: map[string]string{vaNodeIDAnnotation: nodeID},
		},
		Spec: storagev1.VolumeAttachmentSpec{
			Attacher: attacher,
			NodeName: "worker-3",
			Source:   storagev1.VolumeAttachmentSource{PersistentVolumeName: &pv},
		},
	}
	if deleting {
		now := metav1.Now()
		va.DeletionTimestamp = &now
		va.Finalizers = []string{"external-attacher/" + attacher}
	}
	return va
}

func mkPV(name, handle string) *corev1.PersistentVolume {
	return &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{Driver: driverName, VolumeHandle: handle},
			},
		},
	}
}

func TestK8sNodeAttachments(t *testing.T) {
	inline := mkVA("va-inline", driverName, reapNode, "", false)
	inline.Spec.Source = storagev1.VolumeAttachmentSource{
		InlineVolumeSpec: &corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{Driver: driverName, VolumeHandle: "h-inline"},
			},
		},
	}
	cs := fake.NewSimpleClientset(
		mkVA("va-live", driverName, reapNode, "pv-live", false),
		mkVA("va-detaching", driverName, reapNode, "pv-detaching", true),
		mkVA("va-other-node", driverName, "worker-4.node", "pv-other-node", false),
		mkVA("va-other-driver", "other.csi.example.com", reapNode, "pv-other-driver", false),
		inline,
		mkPV("pv-live", "h-live"),
		mkPV("pv-detaching", "h-detaching"),
		mkPV("pv-other-node", "h-other-node"),
		mkPV("pv-other-driver", "h-other-driver"),
	)
	a := &k8sNodeAttachments{cs: cs}

	ids, err := a.AttachedVolumeIDs(context.Background(), reapNode)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"h-live", "h-inline"}, ids)
}

func TestK8sNodeAttachmentsMissingPV(t *testing.T) {
	cs := fake.NewSimpleClientset(mkVA("va-live", driverName, reapNode, "pv-gone", false))
	a := &k8sNodeAttachments{cs: cs}
	_, err := a.AttachedVolumeIDs(context.Background(), reapNode)
	require.Error(t, err, "an unresolvable attachment must not read as detached")
}
