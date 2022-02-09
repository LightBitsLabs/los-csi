// Copyright (C) 2022 metal-stack and scaleway authors
// SPDX-License-Identifier: Apache-2.0
package driver

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
)

var (
	cryptsetupCmd     = "cryptsetup"
	defaultLuksHash   = "sha256"
	defaultLuksCipher = "aes-xts-plain64"
	defaultLuksKeyize = "256"
)

func (d *diskUtils) luksFormat(devicePath string, passphrase string) error {
	args := []string{
		"-q",                      // don't ask for confirmation
		"luksFormat",              // format
		"--hash", defaultLuksHash, // hash algorithm
		"--cipher", defaultLuksCipher, // the cipher used
		"--key-size", defaultLuksKeyize, // the size of the encryption key
		devicePath,                 // device to encrypt
		"--key-file", "/dev/stdin", // read the passphrase from stdin
	}

	d.log.Infof("luksFormat", "args", args)
	luksFormatCmd := exec.Command(cryptsetupCmd, args...)
	luksFormatCmd.Stdin = strings.NewReader(passphrase)

	return luksFormatCmd.Run()
}

func (d *diskUtils) luksOpen(devicePath string, mapperFile string, passphrase string) error {
	args := []string{
		"luksOpen",                 // open
		devicePath,                 // device to open
		mapperFile,                 // mapper file in which to open the device
		"--key-file", "/dev/stdin", // read the passphrase from stdin
	}

	d.log.Infof("luksOpen", "args", args)
	luksOpenCmd := exec.Command(cryptsetupCmd, args...)
	luksOpenCmd.Stdin = strings.NewReader(passphrase)

	return luksOpenCmd.Run()
}

func (d *diskUtils) luksClose(mapperFile string) error {
	args := []string{
		"luksClose", // close
		mapperFile,  // mapper file to close
	}

	d.log.Infof("luksClose", "args", args)
	luksCloseCmd := exec.Command(cryptsetupCmd, args...)

	return luksCloseCmd.Run()
}

func (d *diskUtils) luksStatus(mapperFile string) ([]byte, error) {
	args := []string{
		"status",   // status
		mapperFile, // mapper file to get status
	}

	var stdout bytes.Buffer

	d.log.Infof("luksStatus", "args", args)
	luksStatusCmd := exec.Command(cryptsetupCmd, args...)
	luksStatusCmd.Stdout = &stdout

	err := luksStatusCmd.Run()
	if err != nil {
		return nil, err
	}

	return stdout.Bytes(), nil
}

func (d *diskUtils) luksIsLuks(devicePath string) (bool, error) {
	args := []string{
		"isLuks",   // isLuks
		devicePath, // device path to check
	}

	d.log.Infof("luksIsLuks", "args", args)
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
