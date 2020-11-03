// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package wait

import (
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type CondFunc func() (done bool, err error)

type Backoff struct {
	Delay      time.Duration // initial delay between retries
	Factor     float64       // increase/decrease delay by this factor each step
	DelayLimit time.Duration // retry delay upper/lower bound
	Retries    int           // if unsuccessful - abort after this many retries
}

// WithExponentialBackoff repeatedly invokes the function `fn` with
// exponentially growing/shrinking delay between the calls, until `fn` reports
// that it's done, or until it returns an error, or until a specified maximum
// number of retries. in the latter case a somewhat gRPC-specific Status with
// `DeadlineExceeded` error is returned.
//
// see Backoff documentation of individual `opts` field definitions.
//
// if opts.Delay is not positive - no delay will be introduced between fn()
// invocations. otherwise, the delay will be bounded by opts.DelayLimit, if the
// latter is specified.
//
// if opts.Factor is not positive - the delay introduced will be constant, i.e.
// effective factor of 1.0. if 0 < opts.Factor < 1, the delay introduced will
// be shrinking, otherwise it'll be growing.
//
// if opts.DelayLimit is not positive - the delay will be increased/decreased
// by the specified factor on every step. otherwise, the delay between steps
// will be modified by the specified factor up/down to the delay limit.
//
// if opts.Retries is not positive - `fn` will not be invoked at all.
func WithExponentialBackoff(opts Backoff, fn CondFunc) error {
	if opts.Factor <= 0 {
		opts.Factor = 1.0
	}
	if opts.DelayLimit < 0 {
		opts.DelayLimit = 0
	}
	if opts.DelayLimit != 0 &&
		(opts.DelayLimit < opts.Delay && opts.Factor > 1.0 ||
			opts.DelayLimit > opts.Delay && opts.Factor < 1.0) {
		opts.Delay = opts.DelayLimit
	}

	delay := opts.Delay
	for i := 0; i < opts.Retries; i++ {
		if i != 0 {
			time.Sleep(delay)
			delay = time.Duration(opts.Factor * float64(delay))
			if opts.DelayLimit != 0 &&
				opts.Factor > 1.0 && delay > opts.DelayLimit ||
				opts.Factor < 1.0 && delay < opts.DelayLimit {
				delay = opts.DelayLimit
			}
		}
		if ok, err := fn(); err != nil || ok {
			return err
		}
	}

	return status.Error(codes.DeadlineExceeded, "timed out")
}

func WithRetries(retries int, delay time.Duration, fn CondFunc) error {
	opts := Backoff{Delay: delay, Retries: retries}
	return WithExponentialBackoff(opts, fn)
}
