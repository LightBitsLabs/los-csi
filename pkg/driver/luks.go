// Copyright (C) 2022 metal-stack and scaleway authors
// SPDX-License-Identifier: Apache-2.0
package driver

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	guuid "github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

const (
	cryptsetupCmd     = "cryptsetup"
	defaultLuksHash   = "sha256"
	defaultLuksCipher = "aes-xts-plain64"
	defaultLuksKeyize = "256"
	defaultLuksFormat = "luks2"
	defaultLuksNone   = "none"

	diskMapperPath = "/dev/mapper/"

	DefaultLUKSCfgFileName = "luks_config.yaml"
)

// encryptAndOpenDevice encrypts the volume with the given ID with the given passphrase and open it
// If the device is already encrypted (LUKS header present), it will only open the device
func (d *Driver) encryptAndOpenDevice(volUUID guuid.UUID, passphrase string) (string, error) {
	d.log.Debugf("encryptAndOpenDevice volume uuid: %q", volUUID)
	encryptedDevicePath, err := d.getEncryptedDevicePath(volUUID)
	if err != nil {
		return "", err
	}

	if encryptedDevicePath != "" {
		// device is already encrypted and open
		d.log.Debugf("encryptAndOpenDevice volume: %q is already encrypted", volUUID)
		return encryptedDevicePath, nil
	}

	// let's check if the device is already a luks device
	devicePath, err := d.getDevPathByUUID(volUUID)
	if err != nil {
		return "", mkEnoent("error getting device path for volume %s: %s", volUUID, err)
	}

	isLuks, err := d.luksIsLuks(devicePath)
	if err != nil {
		return "", mkEExec("error checking if device %s is a luks device: %s", devicePath, err)
	}
	if !isLuks {
		// need to format the device
		err = d.luksFormat(devicePath, passphrase)
		if err != nil {
			return "", err
		}
	}

	err = d.luksOpen(devicePath, luksMapperFileName(volUUID), passphrase)
	if err != nil {
		return "", err
	}
	return filepath.Join(diskMapperPath, luksMapperFileName(volUUID)), nil
}

func luksMapperFileName(vid guuid.UUID) string {
	return "lb-csi-" + nvmeUUIDPrefix + vid.String()
}

func (d *Driver) resizeEncryptedDevice(volUUID guuid.UUID) error {
	encryptedDevicePath, err := d.getEncryptedDevicePath(volUUID)
	if err != nil {
		return err
	}
	if encryptedDevicePath == "" {
		// something is wrong...
		return mkInternal("device %s not found", encryptedDevicePath)
	}

	err = d.luksResize(encryptedDevicePath)
	if err != nil {
		return mkInternal("error luks resizing %s: %s", encryptedDevicePath, err)
	}

	return nil
}

func (d *Driver) closeEncryptedDevice(volUUID guuid.UUID) error {
	encryptedDevicePath, err := d.getEncryptedDevicePath(volUUID)
	if err != nil {
		return err
	}
	if encryptedDevicePath == "" {
		// something is wrong...
		return mkInternal("device %s not found", encryptedDevicePath)
	}

	err = d.luksClose(encryptedDevicePath)
	if err != nil {
		return mkInternal("error luks closing %s: %s", encryptedDevicePath, err)
	}

	return nil
}

func (d *Driver) getEncryptedDevicePath(volUUID guuid.UUID) (string, error) {
	encryptedDevicePath := diskMapperPath + luksMapperFileName(volUUID)
	// check that the device file handle exists.
	_, err := os.Stat(encryptedDevicePath)
	if err != nil {
		// if the mapped device does not exists on disk, it's not open
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", mkEExec("error checking stat on %s: %w", encryptedDevicePath, err)
	}

	// check that the device is indeed encrypted
	if d.luksStatus(encryptedDevicePath) == false {
		return "", mkInternal("Unexpected host-encrypted volume %s device %s found un-encrypted",
			volUUID, encryptedDevicePath)
	}

	return encryptedDevicePath, nil
}

type luksConfig struct {
	// limit the amount of memory used to create the encrypted device
	// according to https://gitlab.com/cryptsetup/cryptsetup/-/issues/372
	// the memory consumption during luksFormat is calculated dynamically from the total available memory.
	// this can lead to a situation where a encrypted volume is created on a high memory machine,
	// but cannot opened anymore on a machine with less memory.
	// limit the memory to 64M, given value is kb according to luksFormat help
	PbkdfMemory int64 `yaml:"pbkdfMemory,omitempty"`
}

func loadLuksConfig(log *logrus.Entry, luksCfgFile string) (*luksConfig, error) {
	// setting default values that may be overridden by content of config file
	luksCfg := &luksConfig{
		// 64MB as sane default.
		PbkdfMemory: 65535,
	}

	rawCfg, err := os.ReadFile(luksCfgFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read luks config: %s", err)
		}
		log.Infof("missing luks config file '%s', falling back to default '%v'",
			luksCfgFile, luksCfg)
		return luksCfg, nil
	}

	if err := yaml.Unmarshal(rawCfg, luksCfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal luks config: %s", err)
	}
	return luksCfg, nil
}

// Luks helper

func (d *Driver) luksFormat(devicePath string, passphrase string) error {
	luksCfg, err := loadLuksConfig(d.log, d.luksCfgFile)
	if err != nil {
		return mkExternal("luks config provided but is malformed or can't be parsed: %s", err)
	}

	args := []string{
		"-q",                          // don't ask for confirmation
		"--type=" + defaultLuksFormat, // LUKS2 is default but be explicit
		"--hash", defaultLuksHash,     // hash algorithm
		"--cipher", defaultLuksCipher, // the cipher used
		"--key-size", defaultLuksKeyize, // the size of the encryption key
		"--key-file", "/dev/stdin", // read the passphrase from stdin
		// limit the amount of memory used to create the encrypted device
		// according to https://gitlab.com/cryptsetup/cryptsetup/-/issues/372
		// the memory consumption during luksFormat is calculated dynamically from the total available memory.
		// this can lead to a situation where a encrypted volume is created on a high memory machine,
		// but cannot opened anymore on a machine with less memory.
		// limit the memory to 64M, given value is kb according to luksFormat help
		fmt.Sprintf("--pbkdf-memory=%d", luksCfg.PbkdfMemory),
		"luksFormat", // format
		devicePath,   // device to encrypt
	}

	if !isAESSupported() {
		return mkExternal("your cpu does not support aes")
	}

	d.log.Debugf("luksFormat with args:%v", args)
	cmd := exec.Command(cryptsetupCmd, args...)
	cmd.Stdin = strings.NewReader(passphrase)

	return cmd.Run()
}

func (d *Driver) luksOpen(devicePath string, mapperFile string, passphrase string) error {
	args := []string{
		"luksOpen",          // open
		devicePath,          // device to open
		mapperFile,          // mapper file in which to open the device
		"--disable-keyring", // LUKS2 volumes will ask for passphrase on resize if it is LUKS2 format
		// and if the keyring is not disabled on open
		"--key-file", "/dev/stdin", // read the passphrase from stdin
		// some performance flags - cryptsetup will retry without them
		// if they are unsupported by the kernel
		"--perf-same_cpu_crypt",
		"--perf-submit_from_crypt_cpus",
		"--perf-no_read_workqueue",
		"--perf-no_write_workqueue",
	}

	if !isAESSupported() {
		return mkExternal("your cpu does not support aes")
	}

	d.log.Debugf("luksOpen with args:%v", args)
	cmd := exec.Command(cryptsetupCmd, args...)
	cmd.Stdin = strings.NewReader(passphrase)
	stdout, err := cmd.CombinedOutput()
	if err != nil {
		return mkEExec("luksOpen out:%s error:%v "+
			"(double-check that passphrases match)", string(stdout), err)
	}
	return nil
}

func (d *Driver) luksResize(devicePath string) error {
	args := []string{
		"resize",
		devicePath,
	}
	if !isAESSupported() {
		return mkExternal("your cpu does not support aes")
	}

	d.log.Debugf("resize with args:%v", args)
	out, err := exec.Command(cryptsetupCmd, args...).CombinedOutput()
	if err != nil {
		msg := mkEExec("unable to resize %s with output:%s error:%v", devicePath, string(out), err)
		d.log.Error(msg)
		return msg
	}
	return nil
}

func (d *Driver) luksClose(mapperFile string) error {
	args := []string{
		"luksClose", // close
		mapperFile,  // mapper file to close
	}

	d.log.Debugf("luksClose with args:%v", args)
	cmd := exec.Command(cryptsetupCmd, args...)

	return cmd.Run()
}

// luksStatus returns true if mapperFile is active, otherwise false
func (d *Driver) luksStatus(mapperFile string) bool {
	args := []string{
		"status",   // status
		mapperFile, // mapper file to get status
	}
	d.log.Debugf("luksStatus with args:%v", args)
	cmd := exec.Command(cryptsetupCmd, args...)

	stdout, _ := cmd.CombinedOutput()
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
	cmd := exec.Command(cryptsetupCmd, args...)

	err := cmd.Run()
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

func isAESSupported() bool {
	b, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return false
	}

	for _, line := range strings.Split(string(b), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		value := parts[1]
		switch strings.TrimSpace(key) {
		case "flags":
			flags := strings.Fields(value)
			return contains(flags, "aes")
		}
	}
	return false
}

func contains(elems []string, v string) bool {
	for _, s := range elems {
		if v == s {
			return true
		}
	}
	return false
}
