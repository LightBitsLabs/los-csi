// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"os"
	"path/filepath"
	"testing"

	guuid "github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

const testMapperName = "lb-csi-nvme-uuid.680208b3-4952-4a6c-be3a-348b35afd2f2"

type fakeDM struct {
	dev    string
	name   string
	slaves []string
}

func mkFakeSysfs(t *testing.T, blockDevs []string, dms []fakeDM) string {
	t.Helper()
	root := t.TempDir()
	for _, dev := range blockDevs {
		require.NoError(t, os.MkdirAll(filepath.Join(root, "block", dev), 0o755))
	}
	for _, dm := range dms {
		dmDir := filepath.Join(root, "block", dm.dev)
		require.NoError(t, os.MkdirAll(filepath.Join(dmDir, "dm"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(dmDir, "slaves"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dmDir, "dm", "name"), []byte(dm.name+"\n"), 0o644))
		for _, slave := range dm.slaves {
			require.NoError(t, os.Symlink(filepath.Join(root, "block", slave),
				filepath.Join(dmDir, "slaves", slave)))
		}
	}
	return root
}

func TestInspectLuksMapping(t *testing.T) {
	testCases := []struct {
		name      string
		blockDevs []string
		dms       []fakeDM
		devName   string
		state     luksMappingState
	}{
		{
			name:      "no mappings",
			blockDevs: []string{"nvme0n1"},
			devName:   "nvme0n1",
			state:     luksMappingAbsent,
		},
		{
			name:      "only mappings of other volumes",
			blockDevs: []string{"nvme0n1", "nvme0n2"},
			dms:       []fakeDM{{dev: "dm-0", name: "lb-csi-nvme-uuid.other", slaves: []string{"nvme0n2"}}},
			devName:   "nvme0n1",
			state:     luksMappingAbsent,
		},
		{
			name:      "mapping backed by the volume device",
			blockDevs: []string{"nvme0n1"},
			dms:       []fakeDM{{dev: "dm-0", name: testMapperName, slaves: []string{"nvme0n1"}}},
			devName:   "nvme0n1",
			state:     luksMappingHealthy,
		},
		{
			name:      "backing device removed and volume re-attached under a new name",
			blockDevs: []string{"nvme0n2"},
			dms:       []fakeDM{{dev: "dm-0", name: testMapperName, slaves: []string{"nvme0n1"}}},
			devName:   "nvme0n2",
			state:     luksMappingStale,
		},
		{
			name:    "backing device removed under the same name",
			dms:     []fakeDM{{dev: "dm-0", name: testMapperName, slaves: []string{"nvme0n1"}}},
			devName: "nvme0n1",
			state:   luksMappingStale,
		},
		{
			name:      "mapping backed by a live device of another volume",
			blockDevs: []string{"nvme0n1", "nvme0n2"},
			dms:       []fakeDM{{dev: "dm-0", name: testMapperName, slaves: []string{"nvme0n1"}}},
			devName:   "nvme0n2",
			state:     luksMappingStale,
		},
		{
			name:      "mapping without a backing device",
			blockDevs: []string{"nvme0n1"},
			dms:       []fakeDM{{dev: "dm-0", name: testMapperName}},
			devName:   "nvme0n1",
			state:     luksMappingStale,
		},
		{
			name:      "mapping found among others",
			blockDevs: []string{"nvme0n1", "nvme0n2"},
			dms: []fakeDM{
				{dev: "dm-0", name: "lb-csi-nvme-uuid.other", slaves: []string{"nvme0n2"}},
				{dev: "dm-1", name: testMapperName, slaves: []string{"nvme0n1"}},
			},
			devName: "nvme0n1",
			state:   luksMappingHealthy,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			root := mkFakeSysfs(t, tc.blockDevs, tc.dms)
			state, err := inspectLuksMapping(root, testMapperName, tc.devName)
			require.NoError(t, err)
			require.Equal(t, tc.state, state)
		})
	}
}

func TestCloseEncryptedDeviceWithoutMapping(t *testing.T) {
	d := &Driver{
		log:       logrus.NewEntry(logrus.New()),
		sysfsRoot: mkFakeSysfs(t, []string{"nvme0n1"}, nil),
	}
	vid := guuid.MustParse("680208b3-4952-4a6c-be3a-348b35afd2f2")
	require.NoError(t, d.closeEncryptedDevice(vid))
}

func TestGetEncryptedDevicePath(t *testing.T) {
	vid := guuid.MustParse("680208b3-4952-4a6c-be3a-348b35afd2f2")
	testCases := []struct {
		name      string
		blockDevs []string
		dms       []fakeDM
		devPath   string
		errStr    string
	}{
		{
			name:      "healthy mapping",
			blockDevs: []string{"nvme0n1"},
			dms:       []fakeDM{{dev: "dm-0", name: testMapperName, slaves: []string{"nvme0n1"}}},
			devPath:   "/dev/nvme0n1",
		},
		{
			name:      "no mapping",
			blockDevs: []string{"nvme0n1"},
			devPath:   "/dev/nvme0n1",
			errStr:    "has no open mapping",
		},
		{
			name:      "stale mapping",
			blockDevs: []string{"nvme0n2"},
			dms:       []fakeDM{{dev: "dm-0", name: testMapperName, slaves: []string{"nvme0n1"}}},
			devPath:   "/dev/nvme0n2",
			errStr:    "is not backed by /dev/nvme0n2",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d := &Driver{sysfsRoot: mkFakeSysfs(t, tc.blockDevs, tc.dms)}
			path, err := d.getEncryptedDevicePath(vid, tc.devPath)
			if tc.errStr != "" {
				require.ErrorContains(t, err, tc.errStr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, diskMapperPath+testMapperName, path)
		})
	}
}
