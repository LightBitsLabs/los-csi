// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"fmt"
	"syscall"

	"github.com/sirupsen/logrus"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func isStatusNotFound(err error) bool {
	st, ok := status.FromError(err)
	if ok && st.Code() == codes.NotFound {
		return true
	}
	return false
}

func shouldRetryOn(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	switch st.Code() { //nolint:exhaustive
	case codes.DeadlineExceeded,
		codes.Aborted,
		codes.ResourceExhausted,
		codes.FailedPrecondition, // TODO: hmm... really?
		codes.Unavailable:
		return true
	}
	return false
}

func mungeLBErr(
	log *logrus.Entry, err error, format string, args ...interface{},
) error {
	if shouldRetryOn(err) {
		log.Warnf(format+": "+err.Error(), args...)
		return mkEagain("temporarily "+format, args...)
	}
	return mkExternal(format+": "+err.Error(), args...)
}

// prefixErr() returns an error with the same gRPC error code as `err`, but
// with a message prefixed with an arbitrary blurb.
func prefixErr(err error, format string, args ...interface{}) error {
	st, ok := status.FromError(err)
	if !ok {
		return mkExternal(format+": "+err.Error(), args...)
	}
	return status.Errorf(st.Code(), format+": "+st.Message(), args...)
}

// failure to attach the error details to gRPC response is highly unlikely to
// be spurious runtime error, so tank instead of hiding the real error we were
// trying to report:
func nilOrDie(err error) {
	if err != nil {
		panic(fmt.Sprintf("failed attaching gRPC error details: '%v'", err))
	}
}

func mkEinval(field, msg string) error {
	return status.Errorf(codes.InvalidArgument, "bad value of '%s': %s", field, msg)
}

func mkEinvalMissing(field string) error {
	return status.Errorf(codes.InvalidArgument, "value of '%s' is missing", field)
}

func mkEinvalf(field string, format string, args ...interface{}) error {
	return status.Errorf(codes.InvalidArgument,
		"bad value of '%s': "+format, append([]interface{}{field}, args...)...)
}

func mkEbadOp(kind, subj string, format string, args ...interface{}) error {
	st, err := status.New(codes.FailedPrecondition, "bad operation").WithDetails(
		&errdetails.PreconditionFailure{
			Violations: []*errdetails.PreconditionFailure_Violation{{
				Type:        kind,
				Subject:     subj,
				Description: fmt.Sprintf(format, args...),
			}},
		})
	nilOrDie(err)
	return st.Err()
}

func mkEExec(format string, args ...interface{}) error {
	return status.Errorf(codes.Unknown, "OS error: "+format, args...)
}

func mkEExecOsErr(errno syscall.Errno, desc string) error {
	return mkEExec("%s (%d)", desc, errno)
}

func mkInternal(format string, args ...interface{}) error {
	return status.Errorf(codes.Internal, format, args...)
}

func mkExternal(format string, args ...interface{}) error {
	return status.Errorf(codes.Unknown, format, args...)
}

func mkEExist(format string, args ...interface{}) error {
	return status.Errorf(codes.AlreadyExists, format, args...)
}

func mkEagain(format string, args ...interface{}) error {
	return status.Errorf(codes.Unavailable, format, args...)
}

func mkEnoent(format string, args ...interface{}) error {
	return status.Errorf(codes.NotFound, format, args...)
}

func mkPrecond(format string, args ...interface{}) error {
	return status.Errorf(codes.FailedPrecondition, format, args...)
}

func mkErange(format string, args ...interface{}) error {
	return status.Errorf(codes.OutOfRange, format, args...)
}

func mkAbort(format string, args ...interface{}) error {
	return status.Errorf(codes.Aborted, format, args...)
}
