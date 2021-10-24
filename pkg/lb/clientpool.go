// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package lb

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lightbitslabs/los-csi/pkg/util/endpoint"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type poolMember struct {
	dialCtx  context.Context    // dialling context only, ignored afterwards.
	cancel   context.CancelFunc // to abort blocking dialling prematurely.
	dialDone chan struct{}      // ...successfully or otherwise!

	// all of the below are protected by mu.
	mu sync.Mutex

	clnt     Client // if nil: we're still trying to connect.
	rc       uint64
	expireBy time.Time // zero Time means "never" (in active use).

	// dialErr is mutually exclusive with clnt once dialDone is closed. it
	// may contain errors that are already `status` based, but in addition
	// some generic errors like: context.DeadlineExceeded, context.Canceled,
	// transport.ConnectionError, etc.
	dialErr error

	// set to true by the reaper if the reason for this client being in the
	// process of closing is reaping of an expired but otherwise healthy
	// client. if GetClient() caught it just at the moment of being reaped
	// it will transparently retry, causing redialling.
	reaped bool
}

// ClientPool options defaults
const (
	DefaultClientPoolDialTimeout = 10 * time.Second
	DefaultClientPoolLingerTime  = 10 * time.Minute
	DefaultClientPoolReapCycle   = time.Minute
)

// ClientPoolOptions defines ClientPool operational behaviour
type ClientPoolOptions struct {
	// callers are expected to occasionally retry if dialling fails,
	// normally at the behest of higher-level business logic.
	// default: `lb.DefaultClientPoolDialTimeout`
	DialTimeout time.Duration

	// LB client is not immediately closed when the last active user
	// returns it to the pool, but are left in live state for
	// approximately `LingerTime` for future reuse. after that they
	// are harvested at some point by the reaper.
	// default: `DefaultClientPoolLingerTime`
	LingerTime time.Duration

	// approximately once every `ReapCycle` LB clients that are
	// past their `LingerTime` will be harvested.
	// default: `DefaultClientPoolReapCycle`
	ReapCycle time.Duration
}

type DialFunc func(context.Context, endpoint.Slice, string) (Client, error)

// ClientPool maintains a pool of long-lived LB clients that can be reused
// across individual RPC invocations to avoid connection/authentication
// overheads. only one live client per target is kept.
type ClientPool struct {
	opts ClientPoolOptions

	dialCtx context.Context    // controls dialling only.
	cancel  context.CancelFunc // to abort in-flight dialling on Close().
	dialWG  sync.WaitGroup     // all outstanding dial attempts.
	dialer  DialFunc

	killReaper chan struct{} // close all clients, kill the reaper
	reaperDone chan struct{}

	mu     sync.Mutex             // all of the below are protected by mu.
	pool   map[string]*poolMember // targets -> client
	lut    map[string]*poolMember // client ID -> client
	closed bool
}

// NewClientPoolWithOptions creates and sets up a LB client pool using the
// timeouts specified in `opts`. uninitialised values in `opts` are set to
// defaults.
func NewClientPoolWithOptions(dialer DialFunc, opts ClientPoolOptions) *ClientPool {
	if opts.DialTimeout == 0 {
		opts.DialTimeout = DefaultClientPoolDialTimeout
	}
	if opts.LingerTime == 0 {
		opts.LingerTime = DefaultClientPoolLingerTime
	}
	if opts.ReapCycle == 0 {
		opts.ReapCycle = DefaultClientPoolReapCycle
	}

	cp := &ClientPool{
		opts:       opts,
		dialer:     dialer,
		pool:       make(map[string]*poolMember),
		lut:        make(map[string]*poolMember),
		killReaper: make(chan struct{}),
		reaperDone: make(chan struct{}),
	}
	cp.dialCtx, cp.cancel = context.WithCancel(context.Background())
	go cp.reaper()

	return cp
}

// NewClientPool creates and sets up a LB client pool with default config.
// see NewClientPoolWithOptions() for a more flexible version.
func NewClientPool(dialer DialFunc) *ClientPool {
	return NewClientPoolWithOptions(dialer, ClientPoolOptions{})
}

// Close wraps up the LB client pool operation. it is a blocking call that
// will invoke Close() each of the individual clients and wait for those
// calls to return.
func (cp *ClientPool) Close() {
	cp.mu.Lock()
	if cp.closed {
		cp.mu.Unlock()
		return
	}
	cp.closed = true
	cp.mu.Unlock()

	// first, get all in-flight dial attempts out of the way:
	cp.cancel()
	cp.dialWG.Wait()

	// ...then close the rest:
	close(cp.killReaper)
	<-cp.reaperDone

	cp.mu.Lock()
	leftPool := len(cp.pool)
	leftLut := len(cp.lut)
	cp.mu.Unlock()
	if leftPool != 0 || leftLut != 0 {
		panic(fmt.Sprintf("Close(): %d clients left in the pool, %d in the LUT "+
			"after reaper retired", leftPool, leftLut))
	}
}

// reapClients() reaps only the clients past their best-by date and clients
// that failed dialling altogether.
func (cp *ClientPool) reapClients() {
	cp.mu.Lock()
	for id, pm := range cp.lut {
		pm.mu.Lock()
		// zero best-by date with rc of 0 means pm is still dialling, skip it...
		if pm.rc == 0 && pm.expireBy.Before(time.Now()) && !pm.expireBy.IsZero() {
			clnt := pm.clnt
			if clnt == nil {
				panic(fmt.Sprintf("reapClients(): found expired nil client "+
					"with ID '%s'", id))
			}

			delete(cp.lut, id)
			delete(cp.pool, clnt.Targets())
			cp.mu.Unlock()

			// if GetClient() managed to catch this client right at
			// the moment of being reaped - let it transparently
			// re-create a fresh one instead:
			pm.reaped = true

			pm.clnt = nil
			pm.dialErr = status.Error(codes.Canceled, "LB client connection is closing")
			pm.mu.Unlock()

			clnt.Close()
		} else {
			cp.mu.Unlock()
			pm.mu.Unlock()
		}
		cp.mu.Lock()
	}
	cp.mu.Unlock()
}

// closeClients() unconditionally closes all clients in the pool
func (cp *ClientPool) closeClients() {
	cp.mu.Lock()
	for id, pm := range cp.lut {
		delete(cp.lut, id)
		pm.mu.Lock()
		cp.mu.Unlock()

		clnt := pm.clnt
		if clnt == nil {
			panic(fmt.Sprintf("closeClients(): found nil client with ID '%s'", id))
		}

		tgts := clnt.Targets()
		pm.clnt = nil
		pm.dialErr = status.Error(codes.Canceled, "LB client connection is closing")
		pm.mu.Unlock()
		clnt.Close()

		cp.mu.Lock()
		delete(cp.pool, tgts)
	}
	cp.mu.Unlock()
	close(cp.reaperDone)
}

// reaper() periodically kills off clients that haven't been used for
// at least a while (`cp.opts.LingerTime`).
func (cp *ClientPool) reaper() {
	for {
		select {
		case <-time.After(cp.opts.ReapCycle):
			cp.reapClients()
		case <-cp.killReaper:
			cp.closeClients()
			return
		}
	}
}

// dial() is designed to be called in a dedicated goroutine so that dialling
// can happen in the background, asynchronously from any GetClient() calls
// that may have triggered it or waiting for its results. some, or even all,
// of these GetClient() invocations may not wait until it's done, and time
// out or get cancelled prematurely. in that case the resultant client will
// still end up in the pool.
func (cp *ClientPool) dial(targets endpoint.Slice, mgmtScheme string, pm *poolMember) {
	clnt, err := cp.dialer(pm.dialCtx, targets, mgmtScheme)
	pm.mu.Lock()
	if err != nil {
		clnt = nil // in case dialer was silly...
		pm.dialErr = err
	} else {
		pm.clnt = clnt
		pm.expireBy = time.Now().Add(cp.opts.LingerTime)
	}
	pm.mu.Unlock()

	pm.cancel()

	cp.mu.Lock()
	if clnt != nil {
		// from here on - fear the reaper (well, once cp is unlocked)
		cp.lut[clnt.ID()] = pm
	} else {
		// don't keep clients that failed to connect in the pool:
		delete(cp.pool, targets.String())
	}
	cp.mu.Unlock()
	close(pm.dialDone)
}

// GetClient returns a ready-to-use LB client that can be used to control
// `target` LB. if none exist at the time of invocation, one will be created
// and connected to `target` transparently. GetClient() might block for the
// duration of dialling - within the constraints of the global pool dial
// timeout and `ctx` passed in (timeout, cancellation). this `ctx` has effect
// only on dialling, subsequent individual requests to the client itself take
// their own contexts.
//
// clients obtained from the pool using GetClient() must be returned to the
// pool using PutClient() once the caller is done using them.
func (cp *ClientPool) GetClient(ctx context.Context, targets endpoint.Slice, mgmtScheme string) (Client, error) {
	if !targets.IsValid() {
		return nil, status.Errorf(codes.InvalidArgument,
			"invalid target endpoints specified: [%s]", targets)
	}

retry:
	cp.mu.Lock()
	if cp.closed {
		cp.mu.Unlock()
		return nil, status.Error(codes.Canceled, "LB clients pool is closing")
	}

	tgts := targets.String()
	pm, ok := cp.pool[tgts]

	// no LB client for target yet - create one:
	if !ok {
		pm = &poolMember{
			dialDone: make(chan struct{}),
		}
		pm.dialCtx, pm.cancel = context.WithTimeout(cp.dialCtx, cp.opts.DialTimeout)

		// from here on - it's externally searchable, even if a just dud
		cp.pool[tgts] = pm
		cp.dialWG.Add(1)
		go func() {
			defer cp.dialWG.Done()
			cp.dial(targets, mgmtScheme, pm)
		}()
	}
	cp.mu.Unlock()

	// whether triggered by this call or some other caller, wait for dialling
	// to complete (or fail!) before returning the client - or time out:
	select {
	case <-ctx.Done():
		err := ctx.Err()
		switch err {
		case context.Canceled:
			return nil, status.Error(codes.Canceled, "context cancelled")
		case context.DeadlineExceeded:
			return nil, status.Errorf(codes.DeadlineExceeded,
				"timed out while connecting to LB")
		default:
			return nil, status.Errorf(codes.Unknown,
				"call to LB aborted for unexpected reason: %s", err)
		}
	case <-pm.dialDone:
		pm.mu.Lock()
		dialErr := pm.dialErr
		reaped := pm.reaped

		if dialErr != nil {
			pm.mu.Unlock()
			// it's possible that reaper is right in the middle of
			// expiring this client. make this transparent by retrying...
			if reaped {
				goto retry
			}
			return nil, dialErr
		}

		pm.rc++
		pm.expireBy = time.Time{} // never, it's in use
		defer pm.mu.Unlock()

		return pm.clnt, nil
	}
}

// PutClient returns a client that necessarily must have been previously
// obtained from the receiver pool back to the pool.
func (cp *ClientPool) PutClient(c Client) {
	if c == nil {
		return
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()
	// too late for that, reaper will unconditionally close clients:
	if cp.closed {
		return
	}

	cid := c.ID()
	tgts := c.Targets()
	pm, ok := cp.lut[cid]
	if !ok {
		panic(fmt.Sprintf("PutClient(): client ID '%s' for targets '%s' does "+
			"not belong to this pool", cid, tgts))
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.rc--
	if pm.rc == 0 {
		pm.expireBy = time.Now().Add(cp.opts.LingerTime)
	} else if pm.rc < 0 {
		panic(fmt.Sprintf("PutClient(): negative refcount of %d on "+
			"client ID '%s' for targets '%s'", pm.rc, cid, tgts))
	}
}
