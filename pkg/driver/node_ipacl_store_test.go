// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func mkStoredVolList(t *testing.T) *ipACLVolList {
	return &ipACLVolList{dir: filepath.Join(t.TempDir(), "ipacl")}
}

func TestIPAclVolListSurvivesRestart(t *testing.T) {
	l := mkStoredVolList(t)
	vid1 := mkRecheckVid(t, "10.0.0.1:443,10.0.0.2:443")
	vid1.hostCrypto = defaultLuksFormat
	vid2 := mkRecheckVid(t, "10.0.0.1:443")
	require.NoError(t, l.track(vid1, "jwt1", []string{"10.0.0.5"}))
	require.NoError(t, l.track(vid2, "", nil))

	fi, err := os.Stat(l.dir)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), fi.Mode().Perm())
	fi, err = os.Stat(l.statePath(vid1.uuid))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), fi.Mode().Perm(), "the entry holds a JWT")

	restarted := &ipACLVolList{dir: l.dir}
	n, err := restarted.load()
	require.NoError(t, err)
	require.Equal(t, 2, n)
	got := map[string]*ipACLVol{}
	for _, v := range restarted.list() {
		got[v.vid.uuid.String()] = v
	}
	require.Equal(t, vid1.String(), got[vid1.uuid.String()].vid.String())
	require.Equal(t, "jwt1", got[vid1.uuid.String()].jwt)
	require.Nil(t, got[vid1.uuid.String()].addrs, "the first re-check must compare against the volume")
	require.Equal(t, vid2.String(), got[vid2.uuid.String()].vid.String())
	require.Empty(t, got[vid2.uuid.String()].jwt)
}

func TestIPAclVolListRemovesSavedEntry(t *testing.T) {
	l := mkStoredVolList(t)
	vid1 := mkRecheckVid(t, "10.0.0.1:443")
	vid2 := mkRecheckVid(t, "10.0.0.1:443")
	require.NoError(t, l.track(vid1, "jwt", nil))
	require.NoError(t, l.track(vid2, "jwt", nil))

	require.NoError(t, l.untrack(vid1.uuid))
	require.NoError(t, l.untrack(vid1.uuid), "a retried unstage must succeed")
	require.NoFileExists(t, l.statePath(vid1.uuid))

	stale := l.list()[0]
	require.NoError(t, l.track(vid2, "jwt2", nil))
	require.NoError(t, l.drop(stale))
	require.FileExists(t, l.statePath(vid2.uuid), "a stale re-check result must not remove a newer entry")
	require.NoError(t, l.drop(l.list()[0]))
	require.NoFileExists(t, l.statePath(vid2.uuid))
}

func TestIPAclVolListLoad(t *testing.T) {
	l := mkStoredVolList(t)
	n, err := l.load()
	require.NoError(t, err, "a node that never staged has no state dir")
	require.Zero(t, n)

	good := mkRecheckVid(t, "10.0.0.1:443")
	require.NoError(t, l.track(good, "jwt", nil))
	other := mkRecheckVid(t, "10.0.0.1:443")
	noACL := other
	noACL.ipACL = false
	tmp := filepath.Join(l.dir, ".tmp-123")
	for path, data := range map[string]string{
		tmp:                                     `{"jwt":"x"}`,
		filepath.Join(l.dir, "bad.json"):        `{`,
		filepath.Join(l.dir, "unparsable.json"): `{"volumeHandle":"foo"}`,
		l.statePath(other.uuid):                 `{"volumeHandle":"` + noACL.String() + `"}`,
		filepath.Join(l.dir, good.uuid.String()+"x.json"): `{"volumeHandle":"` + good.String() + `"}`,
	} {
		require.NoError(t, os.WriteFile(path, []byte(data), 0o600))
	}

	restarted := &ipACLVolList{dir: l.dir}
	n, err = restarted.load()
	require.Error(t, err)
	require.Equal(t, 1, n, "a bad entry must not stop the others from loading")
	require.Equal(t, good.uuid, restarted.list()[0].vid.uuid)
	require.NoFileExists(t, tmp)
}
