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

const testK8sNode = "worker-1"

func mkVA(name, attacher, node, pv string, attached bool) *storagev1.VolumeAttachment {
	return &storagev1.VolumeAttachment{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: storagev1.VolumeAttachmentSpec{
			Attacher: attacher,
			NodeName: node,
			Source:   storagev1.VolumeAttachmentSource{PersistentVolumeName: &pv},
		},
		Status: storagev1.VolumeAttachmentStatus{Attached: attached},
	}
}

func mkPV(name string, vid lbResourceID, secret string) *corev1.PersistentVolume {
	src := &corev1.CSIPersistentVolumeSource{Driver: driverName, VolumeHandle: vid.String()}
	if secret != "" {
		src.NodeStageSecretRef = &corev1.SecretReference{Name: secret, Namespace: "ns"}
	}
	return &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{CSI: src},
		},
	}
}

func mkJWTSecret(name, jwt string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"},
		Data:       map[string][]byte{jwtSecretKey: []byte(jwt)},
	}
}

func TestLoadIPAclVolsFromK8s(t *testing.T) {
	withSecret := mkRecheckVid(t, "10.0.0.1:443")
	noSecret := mkRecheckVid(t, "10.0.0.1:443")
	noIPACL := mkRecheckVid(t, "10.0.0.1:443")
	noIPACL.ipACL = false
	missingSecret := mkRecheckVid(t, "10.0.0.1:443")
	otherNode := mkRecheckVid(t, "10.0.0.1:443")
	detached := mkRecheckVid(t, "10.0.0.1:443")

	objs := []runtime.Object{
		mkVA("va1", driverName, testK8sNode, "pv1", true), mkPV("pv1", withSecret, "s1"), mkJWTSecret("s1", "j1"),
		mkVA("va2", driverName, testK8sNode, "pv2", true), mkPV("pv2", noSecret, ""),
		mkVA("va3", driverName, testK8sNode, "pv3", true), mkPV("pv3", noIPACL, "s1"),
		mkVA("va4", driverName, testK8sNode, "pv4", true), mkPV("pv4", missingSecret, "gone"),
		mkVA("va5", driverName, "worker-2", "pv5", true), mkPV("pv5", otherNode, "s1"),
		mkVA("va6", driverName, testK8sNode, "pv6", false), mkPV("pv6", detached, "s1"),
		mkVA("va7", "other.csi.driver", testK8sNode, "pv7", true),
	}
	d := &Driver{log: logrus.NewEntry(logrus.New()), k8sNodeName: testK8sNode}

	n, err := d.loadIPAclVolsFromK8s(context.Background(), d.log, fake.NewSimpleClientset(objs...))

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

func TestLoadIPAclVolsFromK8sKeepsStagedAddrs(t *testing.T) {
	vid := mkRecheckVid(t, "10.0.0.1:443")
	d := &Driver{log: logrus.NewEntry(logrus.New()), k8sNodeName: testK8sNode}
	d.ipACLVols.track(vid, "j0", []string{"10.0.0.5"})
	cs := fake.NewSimpleClientset(
		mkVA("va1", driverName, testK8sNode, "pv1", true), mkPV("pv1", vid, "s1"), mkJWTSecret("s1", "j1"))

	_, err := d.loadIPAclVolsFromK8s(context.Background(), d.log, cs)

	require.NoError(t, err)
	vols := d.ipACLVols.list()
	require.Len(t, vols, 1)
	require.Equal(t, []string{"10.0.0.5"}, vols[0].addrs)
	require.Equal(t, "j1", vols[0].jwt)
}
