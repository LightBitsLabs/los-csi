// Copyright (C) 2022 metal-stack and scaleway authors
// SPDX-License-Identifier: Apache-2.0
package driver

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

const (
	cryptsetupCmd     = "cryptsetup"
	defaultLuksHash   = "sha256"
	defaultLuksCipher = "aes-xts-plain64"
	defaultLuksKeyize = "256"

	diskByIDPath     = "/dev/disk/by-id"
	diskMapperPrefix = "nvme-uuid."
	diskMapperPath   = "/dev/mapper/"
)

// encryptAndOpenDevice encrypts the volume with the given ID with the given passphrase and open it
// If the device is already encrypted (LUKS header present), it will only open the device
func (d *Driver) encryptAndOpenDevice(volumeID string, passphrase string) (string, error) {
	d.log.Debugf("encryptAndOpenDevice volumeID:%q", volumeID)
	encryptedDevicePath, err := d.getMappedDevicePath(volumeID)
	if err != nil {
		d.log.Errorf("encryptAndOpenDevice volumeID:%q error getting mapped device %v", volumeID, err)
		return "", err
	}

	if encryptedDevicePath != "" {
		// device is already encrypted and open
		d.log.Debugf("encryptAndOpenDevice volumeID:%q is already encrypted", volumeID)
		return encryptedDevicePath, nil
	}

	// let's check if the device is already a luks device
	devicePath, err := d.getDevicePathForVolume(volumeID)
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
func (d *Driver) closeDevice(volumeID string) error {
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
func (d *Driver) getMappedDevicePath(volumeID string) (string, error) {
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

// getDevicePathForVolume returns the path for the specified volumeID
func (d *Driver) getDevicePathForVolume(volumeID string) (string, error) {
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
		return "", errors.New("device path does not point on a block device")
	}

	return devicePath, nil
}

// Luks helper

func (d *Driver) luksFormat(devicePath string, passphrase string) error {
	args := []string{
		"-q",                      // don't ask for confirmation
		"luksFormat",              // format
		"--hash", defaultLuksHash, // hash algorithm
		"--cipher", defaultLuksCipher, // the cipher used
		"--key-size", defaultLuksKeyize, // the size of the encryption key
		devicePath,                 // device to encrypt
		"--key-file", "/dev/stdin", // read the passphrase from stdin
	}

	d.log.Debugf("luksFormat with args:%v", args)
	luksFormatCmd := exec.Command(cryptsetupCmd, args...)
	luksFormatCmd.Stdin = strings.NewReader(passphrase)

	return luksFormatCmd.Run()
}

func (d *Driver) luksOpen(devicePath string, mapperFile string, passphrase string) error {
	args := []string{
		"luksOpen",                 // open
		devicePath,                 // device to open
		mapperFile,                 // mapper file in which to open the device
		"--key-file", "/dev/stdin", // read the passphrase from stdin
	}

	d.log.Debugf("luksOpen with args:%v", args)
	luksOpenCmd := exec.Command(cryptsetupCmd, args...)
	luksOpenCmd.Stdin = strings.NewReader(passphrase)
	stdout, err := luksOpenCmd.CombinedOutput()
	d.log.Debugf("luksOpen output:%q", string(stdout))
	if err != nil {
		d.log.Errorf("luksOpen error:%v", err)
		return err
	}

	return nil
}

func (d *Driver) luksResize(mapperFile string, passphrase string) error {
	args := []string{
		"resize",
		mapperFile,
		"--key-file", "/dev/stdin", // read the passphrase from stdin
	}

	d.log.Debugf("resize with args:%v", args)
	luksOpenCmd := exec.Command(cryptsetupCmd, args...)
	luksOpenCmd.Stdin = strings.NewReader(passphrase)

	return luksOpenCmd.Run()
}

func (d *Driver) luksClose(mapperFile string) error {
	args := []string{
		"luksClose", // close
		mapperFile,  // mapper file to close
	}

	d.log.Debugf("luksClose with args:%v", args)
	luksCloseCmd := exec.Command(cryptsetupCmd, args...)

	return luksCloseCmd.Run()
}

// luksStatus returns true if mapperFile is active, otherwise false
func (d *Driver) luksStatus(mapperFile string) bool {
	args := []string{
		"status",   // status
		mapperFile, // mapper file to get status
	}
	d.log.Debugf("luksStatus with args:%v", args)
	luksStatusCmd := exec.Command(cryptsetupCmd, args...)

	stdout, _ := luksStatusCmd.CombinedOutput()
	d.log.Debugf("luksStatus output:%q ", string(stdout))

	statusLines := strings.Split(string(stdout), "\n")

	if len(statusLines) == 0 {
		d.log.Error("luksStatus output has 0 lines")
		return false
	}
	// first line should look like
	// /dev/mapper/<name> is active.
	if strings.Contains(statusLines[0], "is active") {
		return true
	}

	return false
}

func (d *Driver) luksIsLuks(devicePath string) (bool, error) {
	args := []string{
		"isLuks",   // isLuks
		devicePath, // device path to check
	}

	d.log.Debugf("luksIsLuks with args:%v", args)
	luksIsLuksCmd := exec.Command(cryptsetupCmd, args...)

	err := luksIsLuksCmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if ok := errors.As(err, &exitErr); ok {
			if exitErr.ExitCode() == 1 { // not a luks device
				return false, nil
			}
		}
		return false, err
	}
	return true, nil
}
