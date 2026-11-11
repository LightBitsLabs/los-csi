// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package grpcutil

import (
	"context"

	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RespDetailInterceptor doesn't log status details of the responses itself, it
// just stashes them in gRPC context for real middleware logger(s) to pick up
// and log as part of the reverse chain traversal.
func RespDetailInterceptor(
	ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
) (resp interface{}, err error) {
	if resp, err = handler(ctx, req); err == nil {
		return resp, nil
	}
	if st, ok := status.FromError(err); ok {
		details := st.Details()
		if len(details) != 0 {
			tags := grpc_ctxtags.Extract(ctx)
			tags.Set("grpc.status.details", details)
		}
	}
	return resp, err
}

func LBCodeToLogrusLevel(code codes.Code) logrus.Level {
	switch code {
	case codes.OK,
		codes.Canceled,
		codes.DeadlineExceeded,
		codes.NotFound,
		codes.Unavailable:
		return logrus.InfoLevel
	case codes.Aborted,
		codes.AlreadyExists,
		codes.FailedPrecondition,
		codes.InvalidArgument,
		codes.OutOfRange,
		codes.PermissionDenied,
		codes.ResourceExhausted,
		codes.Unauthenticated:
		return logrus.WarnLevel
	case codes.DataLoss,
		codes.Internal,
		codes.Unimplemented,
		codes.Unknown:
		return logrus.ErrorLevel
	default:
		return logrus.ErrorLevel
	}
}

func CodeToLogrusLevel(code codes.Code) logrus.Level {
	switch code {
	case codes.OK,
		codes.Canceled:
		return logrus.InfoLevel
	case codes.Aborted,
		codes.AlreadyExists,
		codes.DeadlineExceeded,
		codes.FailedPrecondition,
		codes.InvalidArgument,
		codes.NotFound,
		codes.OutOfRange,
		codes.PermissionDenied,
		codes.ResourceExhausted,
		codes.Unauthenticated,
		codes.Unavailable,
		codes.Unimplemented:
		return logrus.WarnLevel
	case codes.DataLoss,
		codes.Internal,
		codes.Unknown:
		return logrus.ErrorLevel
	default:
		return logrus.ErrorLevel
	}
}

func ErrFromCtxErr(err error) error {
	switch err {
	case context.Canceled:
		return status.Error(codes.Canceled, "context canceled")
	case context.DeadlineExceeded:
		return status.Error(codes.DeadlineExceeded, "context deadline exceeded")
	default:
		return status.Errorf(codes.Unknown, err.Error())
	}
}
