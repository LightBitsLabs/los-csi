// Copyright (C) 2021 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"context"
	"log/slog"
	"sync/atomic"

	guuid "github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/lightbitslabs/los-csi/pkg/lb"
)

type Wrapper struct {
	be Backend

	// callID is used only to correlate log entries. it is atomic and should be incremented
	// upon entry into each `backend.Backend` method.
	callID uint64

	log *slog.Logger

	// TODO: add [optional] lock to avoid races in backends managing
	// external entities that have no integral protection (e.g. SPDK managed
	// through the basic JSON-RPC API).
	//
	// currently the entire LB CSI plugin Node instance is protected by a
	// single huge and aptly named "bdl" mutex that serialises all the
	// relevant CSI API entrypoints. as one of the side effects - this also
	// protects the Backend instances from races.
	//
	// however, once the "bdl" is replaced by something more fine-grained,
	// there will be cases where the granularity of the Node instance
	// locking will not match that required by the various backends.
	//
	// a new NewWrapper() param should specify whether a given Backend being
	// wrapped requires locking or not. backend.RegisterBackend() should
	// also have this param, stashing it in backend.regEntry and later
	// passing it on to NewWrapper() as necessary. this will allow the
	// backends a way to request or avoid locking as they see fit.
}

func NewWrapper(
	beType string, mkBE MakerFn,
	log *slog.Logger, hostNQN string, rawCfg []byte,
) (Backend, error) {
	log = log.With(
		"backend", beType,
		"hostnqn", hostNQN,
	)
	be, err := mkBE(log, hostNQN, rawCfg)
	if err != nil {
		return nil, err
	}
	return &Wrapper{
		be:  be,
		log: log,
	}, nil
}

func (w *Wrapper) Type() string {
	return w.be.Type()
}

type beFn func() *status.Status

func (w *Wrapper) wrapCall(method string, nguid guuid.UUID, f beFn, errMsg string) *status.Status {
	log := w.log.With(
		"method", method,
		"vol-uuid", nguid,
		"call-id", atomic.AddUint64(&w.callID, 1),
	)
	log.Info("entry")

	st := f()
	if st != nil {
		log.With(
			"code", st.Code(),
			"error", st.Message(),
		).Warn(errMsg)
		return st
	}

	log.With("code", codes.OK).Info("exit")
	return nil
}

func (w *Wrapper) LBVolEligible(ctx context.Context, vol *lb.Volume) *status.Status {
	return w.wrapCall("LBVolEligible", vol.UUID, func() *status.Status {
		return w.be.LBVolEligible(ctx, vol)
	}, "volume likely not eligible to be attached")
}

func (w *Wrapper) Attach(
	ctx context.Context, tgtEnv *TargetEnv, nguid guuid.UUID,
) (st *status.Status) {
	return w.wrapCall("Attach", nguid, func() *status.Status {
		return w.be.Attach(ctx, tgtEnv, nguid)
	}, "likely failed to attach volume to node")
}

func (w *Wrapper) Detach(ctx context.Context, nguid guuid.UUID) (st *status.Status) {
	return w.wrapCall("Detach", nguid, func() *status.Status {
		return w.be.Detach(ctx, nguid)
	}, "likely failed to detach volume from node")
}
