package driver

import (
	"errors"
)

var (
	errTargetPathEmpty               = errors.New("target path empty")
	errTargetNotSharedMounter        = errors.New("target is not shared mounter")
	errTargetNotMounterOnRightDevice = errors.New("target is not mounted on the right device")
	errDevicePathIsNotDevice         = errors.New("device path does not point on a block device")
)
