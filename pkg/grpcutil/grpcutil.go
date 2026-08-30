// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package grpcutil

import (
	"context"

	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
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
