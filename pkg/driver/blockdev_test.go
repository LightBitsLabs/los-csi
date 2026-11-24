// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const gib = int64(1) << 30

func writeFakeDevSize(sysRoot, devName string, bytes int64) error {
	devDir := filepath.Join(sysRoot, "block", devName)
	if err := os.MkdirAll(devDir, 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(devDir, "size.tmp")
	sectors := strconv.FormatInt(bytes/sysfsSectorSize, 10) + "\n"
	if err := os.WriteFile(tmp, []byte(sectors), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(devDir, "size"))
}

func setFakeDevSize(t *testing.T, sysRoot, devName string, bytes int64) {
	t.Helper()
	require.NoError(t, writeFakeDevSize(sysRoot, devName, bytes))
}

func TestBlockDevSize(t *testing.T) {
	sysRoot := t.TempDir()
	setFakeDevSize(t, sysRoot, "nvme0n9", 2*gib)
	size, err := blockDevSize(sysRoot, "nvme0n9")
	require.NoError(t, err)
	require.Equal(t, 2*gib, size)
}

func TestWaitForDevSize(t *testing.T) {
	testCases := []struct {
		name     string
		size     int64
		minBytes int64
	}{
		{name: "already grown", size: 3 * gib, minBytes: 3 * gib},
		{name: "larger than requested", size: 4 * gib, minBytes: 3 * gib},
		{name: "no requested size", size: 2 * gib, minBytes: 0},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sysRoot := t.TempDir()
			setFakeDevSize(t, sysRoot, "nvme0n9", tc.size)
			d := &Driver{sysfsRoot: sysRoot}
			require.NoError(t, d.waitForDevSize("/dev/nvme0n9", tc.minBytes))
		})
	}
}

func TestWaitForDevSizeGrowsLater(t *testing.T) {
	sysRoot := t.TempDir()
	setFakeDevSize(t, sysRoot, "nvme0n9", 2*gib)
	d := &Driver{sysfsRoot: sysRoot}

	grown := make(chan error, 1)
	go func() {
		time.Sleep(3 * devSizeWaitDelay)
		grown <- writeFakeDevSize(sysRoot, "nvme0n9", 3*gib)
	}()
	require.NoError(t, d.waitForDevSize("/dev/nvme0n9", 3*gib))
	require.NoError(t, <-grown)
}

func TestWaitForDevSizeTimeout(t *testing.T) {
	sysRoot := t.TempDir()
	setFakeDevSize(t, sysRoot, "nvme0n9", 2*gib)
	d := &Driver{sysfsRoot: sysRoot}
	err := d.waitForDevSize("/dev/nvme0n9", 3*gib)
	require.Equal(t, codes.Unavailable, status.Code(err))
	require.ErrorContains(t, err, "smaller than the requested")
}

func TestWaitForDevSizeMissingDevice(t *testing.T) {
	d := &Driver{sysfsRoot: t.TempDir()}
	err := d.waitForDevSize("/dev/nvme0n9", 3*gib)
	require.Equal(t, codes.Unknown, status.Code(err))
	require.ErrorContains(t, err, "failed to read size")
}
