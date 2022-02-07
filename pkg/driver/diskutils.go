package driver

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	mount "k8s.io/mount-utils"
)

const (
	diskByIDPath = "/dev/disk/by-id"
	// FIXME what to take here
	diskSCWPrefix        = "lb-volume-"
	diskLuksMapperPrefix = "lb-luks-"
	diskLuksMapperPath   = "/dev/mapper/"

	defaultFSType = "ext4"
)

type DiskUtils interface {
	// FormatAndMount tries to mount `devicePath` on `targetPath` as `fsType` with `mountOptions`
	// If it fails it will try to format `devicePath` as `fsType` first and retry
	FormatAndMount(targetPath string, devicePath string, fsType string, mountOptions []string) error

	// MountToTarget tries to mount `sourcePath` on `targetPath` as `fsType` with `mountOptions`
	MountToTarget(sourcePath, targetPath, fsType string, mountOptions []string) error

	// IsBlockDevice returns true if `path` is a block device
	IsBlockDevice(path string) (bool, error)

	// GetDevicePath returns the path for the specified volumeID
	GetDevicePath(volumeID string) (string, error)

	// IsSharedMounted returns true is `devicePath` is shared mounted on `targetPath`
	IsSharedMounted(targetPath string, devicePath string) (bool, error)

	// GetStatfs return the statfs struct for the given path
	GetStatfs(path string) (*unix.Statfs_t, error)

	// Resize resizes the given volumes
	Resize(targetPath string, devicePath string) error

	// EncryptAndOpenDevice encrypts the volume with the given ID with the given passphrase and open it
	// If the device is already encrypted (LUKS header present), it will only open the device
	EncryptAndOpenDevice(volumeID string, passphrase string) (string, error)

	// CloseDevice closes the encrypted device with the given ID
	CloseDevice(volumeID string) error

	// GetMappedDevicePath returns the path on where the encrypted device with the given ID is mapped
	GetMappedDevicePath(volumeID string) (string, error)
}

type diskUtils struct {
	log *logrus.Entry
}

func newDiskUtils(log *logrus.Entry) *diskUtils {
	return &diskUtils{log: log}
}

func (d *diskUtils) EncryptAndOpenDevice(volumeID string, passphrase string) (string, error) {
	encryptedDevicePath, err := d.GetMappedDevicePath(volumeID)
	if err != nil {
		return "", err
	}

	if encryptedDevicePath != "" {
		// device is already encrypted and open
		return encryptedDevicePath, nil
	}

	// let's check if the device is aready a luks device
	devicePath, err := d.GetDevicePath(volumeID)
	if err != nil {
		return "", fmt.Errorf("error getting device path for volume %s: %w", volumeID, err)
	}
	isLuks, err := luksIsLuks(devicePath)
	if err != nil {
		return "", fmt.Errorf("error checking if device %s is a luks device: %w", devicePath, err)
	}

	if !isLuks {
		// need to format the device
		err = luksFormat(devicePath, passphrase)
		if err != nil {
			return "", fmt.Errorf("error formating device %s: %w", devicePath, err)
		}
	}

	err = luksOpen(devicePath, diskLuksMapperPrefix+volumeID, passphrase)
	if err != nil {
		return "", fmt.Errorf("error luks opening device %s: %w", devicePath, err)
	}
	return diskLuksMapperPath + diskLuksMapperPrefix + volumeID, nil
}

func (d *diskUtils) CloseDevice(volumeID string) error {
	encryptedDevicePath, err := d.GetMappedDevicePath(volumeID)
	if err != nil {
		return err
	}

	if encryptedDevicePath != "" {
		err = luksClose(diskLuksMapperPrefix + volumeID)
		if err != nil {
			return fmt.Errorf("error luks closing %s: %w", encryptedDevicePath, err)
		}
	}

	return nil
}

func (d *diskUtils) GetMappedDevicePath(volumeID string) (string, error) {
	mappedPath := diskLuksMapperPath + diskLuksMapperPrefix + volumeID
	_, err := os.Stat(mappedPath)
	if err != nil {
		// if the mapped device does not exists on disk, it's not open
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("error checking stat on %s: %w", mappedPath, err)
	}

	statusStdout, err := luksStatus(diskLuksMapperPrefix + volumeID)
	if err != nil {
		return "", fmt.Errorf("error checking luks status on %s: %w", diskLuksMapperPrefix+volumeID, err)
	}

	statusLines := strings.Split(string(statusStdout), "\n")

	if len(statusLines) == 0 {
		return "", fmt.Errorf("luksStatus stdout have 0 lines")
	}

	// first line should look like
	// /dev/mapper/<name> is active.
	if !strings.HasSuffix(statusLines[0], "is active.") {
		// when a device is not active, an error exit code is thrown
		// something went wrong if we reach here
		return "", fmt.Errorf("luksStatus returned ok, but device %s is not active", diskLuksMapperPrefix+volumeID)
	}

	return mappedPath, nil
}

func (d *diskUtils) FormatAndMount(targetPath string, devicePath string, fsType string, mountOptions []string) error {
	if fsType == "" {
		fsType = defaultFSType
	}

	d.log.Infof("Attempting to mount %s on %s with type %s", devicePath, targetPath, fsType)
	err := d.MountToTarget(devicePath, targetPath, fsType, mountOptions)
	if err != nil {
		d.log.Infof("Mount attempt failed, trying to format device %s with type %s", devicePath, fsType)
		realFsType, fsErr := d.getDeviceType(devicePath)
		if fsErr != nil {
			return fsErr
		}

		if realFsType == "" {
			fsErr = d.formatDevice(devicePath, fsType)
			if fsErr != nil {
				return fsErr
			}
			return d.MountToTarget(devicePath, targetPath, fsType, mountOptions)
		}
		return err
	}
	return nil
}

func (d *diskUtils) MountToTarget(sourcePath, targetPath, fsType string, mountOptions []string) error {
	if fsType == "" {
		fsType = defaultFSType
	}

	mounter := mount.Mounter{}

	return mounter.Mount(sourcePath, targetPath, fsType, mountOptions)
}

func (d *diskUtils) formatDevice(devicePath string, fsType string) error {
	if fsType == "" {
		fsType = defaultFSType
	}

	mkfsPath, err := exec.LookPath("mkfs." + fsType)
	if err != nil {
		return err
	}

	mkfsArgs := []string{devicePath}
	if fsType == "ext4" || fsType == "ext3" {
		mkfsArgs = []string{
			"-F",  // Force mke2fs to create a filesystem
			"-m0", // 0 blocks reserved for the super-user
			devicePath,
		}
	}

	return exec.Command(mkfsPath, mkfsArgs...).Run()
}

func (d *diskUtils) getDeviceType(devicePath string) (string, error) {
	blkidPath, err := exec.LookPath("blkid")
	if err != nil {
		return "", err
	}

	blkidArgs := []string{"-p", "-s", "TYPE", "-s", "PTTYPE", "-o", "export", devicePath}
	blkidOutputBytes, err := exec.Command(blkidPath, blkidArgs...).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 2 {
				// From man page of blkid:
				// If the specified token was not found, or no (specified) devices
				// could be identified, or it is impossible to gather
				// any information about the device identifiers
				// or device content an exit code of 2 is returned.
				return "", nil
			}
		}
		return "", err
	}

	blkidOutput := string(blkidOutputBytes)
	blkidOutputLines := strings.Split(blkidOutput, "\n")
	for _, blkidLine := range blkidOutputLines {
		if len(blkidLine) == 0 {
			continue
		}

		blkidLineSplit := strings.Split(blkidLine, "=")
		if blkidLineSplit[0] == "TYPE" && len(blkidLineSplit[1]) > 0 {
			return blkidLineSplit[1], nil
		}
	}
	// TODO real error???
	return "", nil
}

func (d *diskUtils) GetDevicePath(volumeID string) (string, error) {
	devicePath := path.Join(diskByIDPath, diskSCWPrefix+volumeID)
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
