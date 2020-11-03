// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// errors: -------------------------------------------------------------------

type BadArgError struct {
	Param  string // name of offending func/method param
	Arg    string // human-readable representation of offending value
	Reason string // optional
}

func (e *BadArgError) Error() string {
	if e == nil {
		return "<nil>"
	}
	s := "invalid argument to '" + e.Param + "': '" + e.Arg + "'"
	if e.Reason != "" {
		s += ", " + e.Reason
	}
	return s
}

type OsError struct {
	Errno syscall.Errno
	Op    string
}

func (e *OsError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return e.Op + ": " + e.Errno.Error()
}

// executor: -----------------------------------------------------------------

const errAlreadyStr = "Failed to write to /dev/nvme-fabrics: Operation already in progress\n"

var execOps uint64

func execNvme(log *logrus.Entry, args ...string) (string, error) {
	op := atomic.AddUint64(&execOps, 1)
	log = log.WithField("cmd", op)
	cmd := exec.Command("nvme", args...)
	log.Debugf("CMD: %s", strings.Join(cmd.Args, " "))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	errStr := string(stderr.Next(256))
	outStr := stdout.String()
	var res string
	if err != nil {
		if execErr, ok := err.(*exec.Error); ok {
			log.Warnf("nvme-cli is broken or not installed: %s", execErr.Err)
			return "", execErr
		}

		// err here is normally ExitError, so err.String() is down to
		// os.ProcessState, which will typically yield a goofy
		// 'exit status N'. BUT, it also stringizes a bunch of other
		// interesting conditions, like signals, core being dumped, etc.,
		// so bite the bullet...
		log.WithField(
			"err", errStr,
		).Infof("FAIL: %s", err) // TODO: Debugf? this should be up to the caller!
		res = errStr
	} else {
		// nvme-cli often yields non-fatal error messages, typically
		// when its slightly - but not lethally - out of sync with the
		// kernel idea of what sysfs should look like.
		log.WithFields(logrus.Fields{
			"err": errStr,
			"out": outStr,
		}).Debugf("OK")
		res = outStr
	}
	return res, err
}

// nvmeCliToOsError unsuccessfully attempts to mop up the error code mess left
// over by 'nvme-cli'. some versions of 'nvme-cli' return raw internal errors
// positive and negative alike, or errors received from kernel verbatim. some
// versions attempt to squelch the negative errors by converting them to some
// positive, error codes, but in the process making the error code in question
// worse than meaningless: e.g. converting EALREADY for a connection request
// (which is effectively a success), into an arbitrary ECOMM (which a failure,
// with no way of figuring out the specific type of "error"). and the errors
// returned change literally from minor release to minor release.
func nvmeCliErrToErrno(exitCode int) syscall.Errno {
	if exitCode < 0 {
		return syscall.Errno(-int8(exitCode))
	}
	// TODO: this doesn't work with older versions of nvme-cli that
	// occasionally emitted raw internal errors verbatim...
	return syscall.Errno(exitCode)
}

// nvme-cli wrapper: ---------------------------------------------------------

type Cli struct {
	log *logrus.Entry
}

func New(log *logrus.Entry) (*Cli, error) {
	tmpLog := log.WithField("nvme-of.client", "nvme-cli")

	out, err := execNvme(tmpLog, "version")
	if err != nil {
		return nil, errors.Wrap(err, "nvme-cli is broken or not installed")
	}

	verOrDummy := func(ver string) string {
		if ver == "" {
			ver = "<UNKNOWN>"
		}
		return ver
	}
	ver := verOrDummy(strings.TrimSpace(out))
	ver = verOrDummy(strings.TrimSpace(strings.TrimPrefix(ver, "nvme version ")))
	suffix := ""
	if utf8.RuneCountInString(ver) > 64 {
		suffix = "..."
	}
	ver = fmt.Sprintf("nvme-cli, ver: %.64s%s", ver, suffix)

	return &Cli{log: log.WithField("nvme-of.client", ver)}, nil
}

func (c *Cli) Connect(trtype, subnqn, traddr, trsvcid, hostnqn string) error {
	switch trtype {
	case "tcp", "rdma":
	default:
		return &BadArgError{"trtype", trtype, "not one of: tcp, rdma"}
	}
	if subnqn == "" {
		return &BadArgError{Param: "subnqn"}
	}
	if traddr == "" {
		return &BadArgError{Param: "traddr"}
	}
	// don't bother with defaults for now, force explicit port
	if trsvcid == "" {
		return &BadArgError{Param: "trsvcid"}
	}
	port, err := strconv.Atoi(trsvcid)
	if err != nil || port < 1 || port > 0xFFFF {
		return &BadArgError{Param: "trsvcid", Arg: trsvcid}
	}

	ips, err := net.LookupIP(traddr)
	if err != nil {
		return &BadArgError{"traddr", traddr, err.Error()}
	}
	// TODO: for now - always choose the primary, but consider looping over
	// the secondaries if fail to connect...
	ipAddr := ips[0].String()

	args := []string{
		"connect",
		"-t", trtype,
		"-a", ipAddr,
		"-n", subnqn,
		"-s", trsvcid,
	}
	if hostnqn != "" {
		args = append(args, "-q", hostnqn)
	}

	out, err := execNvme(c.log, args...)
	if err == nil {
		return nil
	}
	errno := syscall.ENOCSI // geddit?
	switch err := err.(type) {
	case *exec.Error:
		return err
	case *exec.ExitError:
		if ws, ok := err.Sys().(syscall.WaitStatus); ok {
			errno = nvmeCliErrToErrno(ws.ExitStatus())
			// different versions of 'nvme-cli' behave differently
			// in this case, returning different errors, so fall
			// back to this ridiculous hack:
			if errno == syscall.EALREADY || out == errAlreadyStr {
				c.log.Infof("already connected to target at %s:%s with SubNQN '%s'",
					ipAddr, trsvcid, subnqn)
				return nil
			}
		}
		return &OsError{Op: "exec 'nvme'", Errno: errno}
	default:
		c.log.Debugf("'nvme connect' failed with output: '%s'", out)
		return &OsError{Op: fmt.Sprintf("exec 'nvme': unknown error: %s",
			err.Error()), Errno: errno}
	}
}

/* TODO: 'nvme-cli' errors observed in the wild:
146: ETIMEDOUT    110  timed out to non-existent target IP
234: EINVAL        22  passing '-t tcp' on an SoftRoCE/RXE-only target
152: ECONNRESET   104  trying to connect to the wrong NIC on target
142: EALREADY     114  when already attached as this HOSTNQN to that SUBNQN
251: EIO            5  when no such SUBNQN exported on a target port with dmesg
                           entry of "Connect Invalid Data Parameter"
                       trying to connect on mgmt port ("timeout" in dmesg)
145: ECONNREFUSED 111  trying to connect on a wrong port
*/

// example `nvme list -o json` output:
//    {
//      "Devices" : [
//        {
//          "DevicePath" : "/dev/nvme1n2",
//          "Firmware" : "0.1",
//          "Index" : 1,
//          "ModelNumber" : "LightBox",
//          "ProductName" : "Unknown Device",
//          "SerialNumber" : "7f17657f0c30",
//          "UsedBytes" : 536870912000,
//          "MaximiumLBA" : 131072000,
//          "PhysicalSize" : 536870912000,
//          "SectorSize" : 4096,
//          "UUID" : "3f338692-9712-43e6-8751-7c3d27c30b24"
//        },
//        {
//             ...
//        }
//      ]
//    }
type nvmeListResponse struct {
	Devices []nvmeDevInfo
}
type nvmeDevInfo struct {
	DevicePath string
	NGUID      string `json:"UUID"`
}

// GetDevPathByNGUID returns the absolute path of a Linux block device that
// has a specified NGUID and nil on success. if search proceeded without failures
// but such device simply does not exist - returns empty string and nil. if any
// error was encountered while trying to search for the device, returns an empty
// string and a corresponding error.
//
// NOTE: this implementation is based on parsing the output of `nvme list -o json`
// which is, unfortunately, quite broken in some of the earlier versions of
// `nvme-cli`(e.g. v1.4-3.el7 from CentOS 7 simply returns about half the
// output - and NO NGUIDs! - because it looks for the wrong files under sysfs
// and just errors out). iterating directly over sysfs might be more of a
// hassle, but is more likely you yield better results. except for that one
// period when kernel reported NGUIDs out of the `uuid` sysfs virtual file,
// until NGUIDs got their own dedicated `nguid` file, though they're still
// occasionally reported as `uuid` - if there isn't a proper UUID, you see,
// for reverse compatibility with the original bug...
//
// TODO: take subsystem NQN as well and check that the device we found does
// belong to the proper controller? `nvme id-ctrl /dev/nvme<bleh> -o json`
// will do the trick. assuming UUIDs are... well... actually "UU" (a thing
// not to be taken for granted, especially in test/dev environments) - that
// shouldn't really be a concern, i suppose, but you can't be too careful.
// speaking of which, then there's the little matter of TOCTTOU on the device
// between these two queries, so maybe best to double check that the `sn` field
// matches `SerialNumber` in `nvme list -o json` output, just in case. which is
// no guarantee, of course, but given the interface we're dealing with here -
// probably as good as it gets.
func (c *Cli) GetDevPathByNGUID(nguid uuid.UUID) (string, error) {
	out, err := execNvme(c.log, "list", "-o", "json")

	if err == exec.ErrNotFound {
		c.log.Debug("nvme-cli is not installed or not in path")
		return "", &OsError{Errno: syscall.ENOENT, Op: "exec 'nvme'"}
	}
	errno := syscall.ENOCSI // whoda thunk, right?
	switch err := err.(type) {
	case *exec.ExitError:
		if ws, ok := err.Sys().(syscall.WaitStatus); ok {
			errno = nvmeCliErrToErrno(ws.ExitStatus())
		}
		return "", &OsError{Op: "exec 'nvme'", Errno: errno}
	default:
		c.log.Debugf("'nvme list' failed with output: '%s'", out)
		return "", &OsError{Op: fmt.Sprintf("exec 'nvme': unknown error: %s",
			err.Error()), Errno: errno}
	case nil:
	}

	// workaround for an nvme-cli bug where `nvme list -o json` produces
	// literally empty output (which is NOT a valid JSON document) instead
	// of something like '{}' when there are no devices to list.
	if strings.TrimSpace(out) == "" {
		return "", nil
	}

	var devList nvmeListResponse
	err = json.Unmarshal([]byte(out), &devList)
	if err != nil {
		return "", &OsError{Op: fmt.Sprintf("parse device list: %s",
			err.Error()), Errno: syscall.EILSEQ} // artistic license
	}

	for _, dev := range devList.Devices {
		devNguid, err := uuid.Parse(dev.NGUID)
		if err != nil {
			c.log.Warningf("'nvme list' seeing devices without valid NGUID")
			continue
		}
		if nguid == devNguid {
			return dev.DevicePath, nil
		}
	}

	return "", nil
}
