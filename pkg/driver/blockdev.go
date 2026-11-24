// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/lightbitslabs/los-csi/pkg/util/wait"
)

const (
	// <sysfs>/block/<dev>/size counts 512-byte sectors, whatever the device's logical block size.
	sysfsSectorSize = 512

	devSizeWaitRetries = 30
	devSizeWaitDelay   = 100 * time.Millisecond
)

func blockDevSize(sysRoot, devName string) (int64, error) {
	raw, err := os.ReadFile(filepath.Join(sysRoot, "block", devName, "size"))
	if err != nil {
		return 0, err
	}
	sectors, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil {
		return 0, err
	}
	return sectors * sysfsSectorSize, nil
}

func (d *Driver) waitForDevSize(devPath string, minBytes int64) error {
	devName := filepath.Base(devPath)
	var size int64
	err := wait.WithRetries(devSizeWaitRetries, devSizeWaitDelay, func() (bool, error) {
		var err error
		size, err = blockDevSize(d.sysfsRoot, devName)
		return size >= minBytes, err
	})
	if status.Code(err) == codes.DeadlineExceeded {
		return mkEagain("device %s is %d bytes, smaller than the requested %d bytes",
			devPath, size, minBytes)
	}
	if err != nil {
		return mkEExec("failed to read size of device %s: %s", devPath, err)
	}
	return nil
}
