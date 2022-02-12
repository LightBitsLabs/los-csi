// Copyright (C) 2022 metal-stack and scaleway authors
// SPDX-License-Identifier: Apache-2.0
package driver

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

const (
	diskByIDPath     = "/dev/disk/by-id"
	diskMapperPrefix = "nvme-uuid."
	diskMapperPath   = "/dev/mapper/"
)

type diskUtils struct {
	log *logrus.Entry
}

func newDiskUtils(log *logrus.Entry) *diskUtils {
	return &diskUtils{log: log}
}

// encryptAndOpenDevice encrypts the volume with the given ID with the given passphrase and open it
// If the device is already encrypted (LUKS header present), it will only open the device
func (d *diskUtils) encryptAndOpenDevice(volumeID string, passphrase string) (string, error) {
	d.log.Infof("encryptAndOpenDevice volumeID:%q", volumeID)
	encryptedDevicePath, err := d.getMappedDevicePath(volumeID)
	if err != nil {
		d.log.Errorf("encryptAndOpenDevice volumeID:%q error getting mapped device %v", volumeID, err)
		return "", err
	}

	if encryptedDevicePath != "" {
		// device is already encrypted and open
		d.log.Infof("encryptAndOpenDevice volumeID:%q is already encrypted", volumeID)
		return encryptedDevicePath, nil
	}

	// let's check if the device is already a luks device
	devicePath, err := d.getDevicePath(volumeID)
	if err != nil {
		return "", fmt.Errorf("error getting device path for volume %s: %w", volumeID, err)
	}
	isLuks, err := d.luksIsLuks(devicePath)
	if err != nil {
		return "", fmt.Errorf("error checking if device %s is a luks device: %w", devicePath, err)
	}

	if !isLuks {
		// need to format the device
		err = d.luksFormat(devicePath, passphrase)
		if err != nil {
			return "", fmt.Errorf("error formating device %s: %w", devicePath, err)
		}
	}

	err = d.luksOpen(devicePath, diskMapperPrefix+volumeID, passphrase)
	if err != nil {
		return "", fmt.Errorf("error luks opening device %s: %w", devicePath, err)
	}
	return diskMapperPath + diskMapperPrefix + volumeID, nil
}

// closeDevice closes the encrypted device with the given ID
func (d *diskUtils) closeDevice(volumeID string) error {
	encryptedDevicePath, err := d.getMappedDevicePath(volumeID)
	if err != nil {
		return err
	}

	if encryptedDevicePath != "" {
		err = d.luksClose(diskMapperPrefix + volumeID)
		if err != nil {
			return fmt.Errorf("error luks closing %s: %w", encryptedDevicePath, err)
		}
	}

	return nil
}

// getMappedDevicePath returns the path on where the encrypted device with the given ID is mapped
func (d *diskUtils) getMappedDevicePath(volumeID string) (string, error) {
	volume := diskMapperPrefix + volumeID
	mappedPath := filepath.Join(diskByIDPath, volume)
	_, err := os.Stat(mappedPath)
	if err != nil {
		// if the mapped device does not exists on disk, it's not open
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("error checking stat on %s: %w", mappedPath, err)
	}

	isActive := d.luksStatus(volume)
	if isActive {
		return mappedPath, nil
	}
	return "", nil
}

// getDevicePath returns the path for the specified volumeID
func (d *diskUtils) getDevicePath(volumeID string) (string, error) {
	volume := diskMapperPrefix + volumeID
	devicePath := path.Join(diskByIDPath, volume)
	realDevicePath, err := filepath.EvalSymlinks(devicePath)
	if err != nil {
		return "", err
	}

	deviceInfo, err := os.Stat(realDevicePath)
	if err != nil {
		return "", err
	}

	deviceMode := deviceInfo.Mode()
	if os.ModeDevice != deviceMode&os.ModeDevice || os.ModeCharDevice == deviceMode&os.ModeCharDevice {
		return "", errDevicePathIsNotDevice
	}

	return devicePath, nil
}
