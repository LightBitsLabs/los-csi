// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package wait_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/lightbitslabs/los-csi/pkg/util/wait"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	ms = time.Millisecond
)

func isDeadlineExceeded(err error) bool {
	st, ok := status.FromError(err)
	if ok && st.Code() == codes.DeadlineExceeded {
		return true
	}
	return false
}

func doBackoffLogicTest(t *testing.T, retries int, shouldPass bool, shouldError bool) {
	opts := wait.Backoff{Retries: retries}
	fnError := fmt.Errorf("fn() errored out")
	n := 0

	err := wait.WithExponentialBackoff(opts, func() (bool, error) {
		n++
		if shouldError {
			return shouldPass, fnError
		}
		if shouldPass {
			return true, nil
		}
		return false, nil
	})
	if retries < 1 {
		if n != 0 {
			t.Errorf("BUG: fn() invoked %d times on %d retries",
				n, opts.Retries)
		}
		if !isDeadlineExceeded(err) {
			t.Errorf("BUG: got unexpected error %+v", err)
		}
	} else {
		if shouldError {
			if n != 1 {
				t.Errorf("BUG: fn() invoked %d times despite "+
					"erroring out on the 1st pass", n)
			}
			if err == nil {
				t.Errorf("BUG: didn't get an errpr despite " +
					"fn() erroring out on the 1st pass")
			}
		} else if shouldPass {
			if n != 1 {
				t.Errorf("BUG: fn() invoked %d times despite "+
					"succeeding on the 1st pass", n)
			}
			if err != nil {
				t.Errorf("BUG: got unexpected error '%+v' despite "+
					"fn() succeeding on the 1st pass", err)
			}
		} else {
			if n != retries {
				t.Errorf("BUG: fn() invoked %d times on %d retries",
					n, retries)
			}
			if !isDeadlineExceeded(err) {
				t.Errorf("BUG: got unexpected error '%+v'", err)
			}
		}
	}
}

func TestBackoffLogic(t *testing.T) {
	retries := []int{-5, 0, 1, 5}
	for _, doErr := range []bool{false, true} {
		for _, pass := range []bool{false, true} {
			for _, r := range retries {
				t.Run(fmt.Sprintf("retries:%d;pass:%t;err:%t", r, pass, doErr),
					func(t *testing.T) {
						doBackoffLogicTest(t, r, pass, doErr)
					})
			}
		}
	}
}

// this is a wild hack, of course, since the delays are barely a lower bound,
// but this should usually pass on a reasonably idle server... ;)
func doBackoffTimeoutTest(t *testing.T, opts wait.Backoff) {
	n := 0
	factor := opts.Factor
	if factor == 0 {
		factor = 1.0
	}
	margin := 10 * ms
	exactDelay := time.Duration(0)
	start := time.Now()

	err := wait.WithExponentialBackoff(opts, func() (bool, error) {
		gap := time.Since(start)
		if gap < exactDelay || gap > exactDelay+margin {
			t.Errorf("BUG: retry %d got delayed %s instead of ~%s",
				n, gap, exactDelay)
		}
		if n == 0 {
			exactDelay = opts.Delay
		} else {
			exactDelay = time.Duration(float64(exactDelay) * factor)
		}
		if opts.DelayLimit != 0 &&
			(factor > 1.0 && exactDelay > opts.DelayLimit ||
				factor < 1.0 && exactDelay < opts.DelayLimit) {
			exactDelay = opts.DelayLimit
		}
		n++
		start = time.Now()
		return false, nil
	})

	if n != opts.Retries {
		t.Errorf("BUG: fn() invoked %d times on %d retries",
			n, opts.Retries)
	}
	if !isDeadlineExceeded(err) {
		t.Errorf("BUG: got unexpected error %+v", err)
	}
}

var testOpts = []wait.Backoff{
	{},
	{Factor: 2.0, DelayLimit: 50 * ms},
	{Delay: 100 * ms},
	{Delay: 100 * ms, DelayLimit: 110 * ms},
	{Delay: 100 * ms, Factor: 1.0},
	{Delay: 100 * ms, Factor: 1.5},
	{Delay: 100 * ms, Factor: 1.5, DelayLimit: 110 * ms},
	{Delay: 100 * ms, Factor: 0.5},
	{Delay: 100 * ms, Factor: 0.5, DelayLimit: 60 * ms},
}

func TestBackoffTimeouts(t *testing.T) {
	for _, opts := range testOpts {
		opts := opts
		opts.Retries = 4
		t.Run(
			fmt.Sprintf("delay:%s;factor:%1.1f;lim:%s;retries:%d",
				opts.Delay, opts.Factor, opts.DelayLimit, opts.Retries),
			func(t *testing.T) {
				doBackoffTimeoutTest(t, opts)
			},
		)
	}
}
