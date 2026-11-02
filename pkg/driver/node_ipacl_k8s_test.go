// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

const testNodeID = "worker-1.node"

func mkAttachedVA(name, nodeID, pv string) *storagev1.VolumeAttachment {
	va := mkVA(name, driverName, nodeID, pv, false)
	va.Status.Attached = true
	return va
}

func mkStagePV(name string, vid lbResourceID, secret string) *corev1.PersistentVolume {
	pv := mkPV(name, vid.String())
	if secret != "" {
		pv.Spec.CSI.NodeStageSecretRef = &corev1.SecretReference{Name: secret, Namespace: "ns"}
	}
	return pv
}

func mkJWTSecret(name, jwt string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"},
		Data:       map[string][]byte{jwtSecretKey: []byte(jwt)},
	}
}

func mkLoadDriver() *Driver {
	return &Driver{log: logrus.NewEntry(logrus.New()), nodeID: testNodeID}
}

func TestLoadIPAclVolsFromK8s(t *testing.T) {
	withSecret := mkRecheckVid(t, "10.0.0.1:443")
	noSecret := mkRecheckVid(t, "10.0.0.1:443")
	noIPACL := mkRecheckVid(t, "10.0.0.1:443")
	noIPACL.ipACL = false
	otherNode := mkRecheckVid(t, "10.0.0.1:443")
	notAttached := mkRecheckVid(t, "10.0.0.1:443")

	objs := []runtime.Object{
		mkAttachedVA("va1", testNodeID, "pv1"), mkStagePV("pv1", withSecret, "s1"), mkJWTSecret("s1", "j1"),
		mkAttachedVA("va2", testNodeID, "pv2"), mkStagePV("pv2", noSecret, ""),
		mkAttachedVA("va3", testNodeID, "pv3"), mkStagePV("pv3", noIPACL, "s1"),
		mkAttachedVA("va4", "worker-2.node", "pv4"), mkStagePV("pv4", otherNode, "s1"),
		mkVA("va5", driverName, testNodeID, "pv5", false), mkStagePV("pv5", notAttached, "s1"),
	}
	d := mkLoadDriver()

	n, err := d.loadIPAclVolsFromK8s(context.Background(), d.log,
		&k8sNodeAttachments{cs: fake.NewSimpleClientset(objs...)})

	require.NoError(t, err)
	require.Equal(t, 2, n)
	got := map[string]string{}
	for _, v := range d.ipACLVols.list() {
		require.Nil(t, v.addrs, "a loaded volume must be enrolled on the first re-check")
		got[v.vid.uuid.String()] = v.jwt
	}
	require.Equal(t, map[string]string{
		withSecret.uuid.String(): "j1",
		noSecret.uuid.String():   "",
	}, got)
}

func TestLoadIPAclVolsFromK8sMissingSecret(t *testing.T) {
	loaded := mkRecheckVid(t, "10.0.0.1:443")
	missing := mkRecheckVid(t, "10.0.0.1:443")
	cs := fake.NewSimpleClientset(
		mkAttachedVA("va1", testNodeID, "pv1"), mkStagePV("pv1", loaded, "s1"), mkJWTSecret("s1", "j1"),
		mkAttachedVA("va2", testNodeID, "pv2"), mkStagePV("pv2", missing, "gone"))
	d := mkLoadDriver()

	n, err := d.loadIPAclVolsFromK8s(context.Background(), d.log, &k8sNodeAttachments{cs: cs})

	require.Error(t, err, "a volume that fails to load must cause a retry")
	require.Equal(t, 1, n)
	vols := d.ipACLVols.list()
	require.Len(t, vols, 1)
	require.Equal(t, loaded.uuid, vols[0].vid.uuid)
}

func TestLoadIPAclVolsFromK8sUnannotatedVA(t *testing.T) {
	vid := mkRecheckVid(t, "10.0.0.1:443")
	va := mkUnannotatedVA("va1", "worker-1", "pv1")
	va.Status.Attached = true
	cs := fake.NewSimpleClientset(va, mkCSINode("worker-1", testNodeID), mkStagePV("pv1", vid, ""))
	d := mkLoadDriver()

	n, err := d.loadIPAclVolsFromK8s(context.Background(), d.log, &k8sNodeAttachments{cs: cs})

	require.NoError(t, err)
	require.Equal(t, 1, n)
}

func TestLoadIPAclVolsFromK8sKeepsStagedAddrs(t *testing.T) {
	vid := mkRecheckVid(t, "10.0.0.1:443")
	d := mkLoadDriver()
	d.ipACLVols.track(vid, "j0", []string{"10.0.0.5"})
	cs := fake.NewSimpleClientset(
		mkAttachedVA("va1", testNodeID, "pv1"), mkStagePV("pv1", vid, "s1"), mkJWTSecret("s1", "j1"))

	_, err := d.loadIPAclVolsFromK8s(context.Background(), d.log, &k8sNodeAttachments{cs: cs})

	require.NoError(t, err)
	vols := d.ipACLVols.list()
	require.Len(t, vols, 1)
	require.Equal(t, []string{"10.0.0.5"}, vols[0].addrs)
	require.Equal(t, "j1", vols[0].jwt)
}
