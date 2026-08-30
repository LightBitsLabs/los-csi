// Copyright (C) 2016--2021 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package dsc

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path"
	"strings"
	"time"

	guuid "github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/lightbitslabs/los-csi/pkg/driver/backend"
	"github.com/lightbitslabs/los-csi/pkg/lb"
)

const (
	beType = "dsc"

	defaultDSCConfigPath = "/etc/discovery-client/discovery.d"
	dscReservedPrefix    = "tmp.dc."
	dscWarnPeriod        = 10 * time.Minute // to avoid log spam
)

type Backend struct {
	hostNQN string

	dscCfgPath      string
	lastDSCWarnTime time.Time

	log *slog.Logger
}

func New(log *slog.Logger, hostNQN string) (*Backend, error) { //nolint:unparam
	be := Backend{
		hostNQN:    hostNQN,
		dscCfgPath: defaultDSCConfigPath,
		log:        log,
	}

	be.log.Info("starting")

	// container start-up order in a pod is undefined (ex. init), so for
	// containerised DSC deployments it might be too early to check for the
	// dscCfgPath existence, even though it would have been handy...

	return &be, nil
}

func init() {
	backend.RegisterBackend(beType,
		func(log *slog.Logger, hostNQN string, rawCfg []byte) (backend.Backend, error) {
			return New(log, hostNQN)
		})
}

func (be *Backend) Type() string { //revive:disable-line:unused-receiver
	return beType
}

func (be *Backend) LBVolEligible( //revive:disable-line:unused-receiver
	_ context.Context, _ *lb.Volume,
) *status.Status {
	return nil
}

func (be *Backend) checkDSCCfgPath() error {
	fi, err := os.Stat(be.dscCfgPath)
	if err != nil || fi == nil || !fi.IsDir() {
		now := time.Now()
		if now.After(be.lastDSCWarnTime.Add(dscWarnPeriod)) {
			be.log.Error("can't communicate with the DSC through config dir make sure the Discovery Service Client is properly configured and running on this node", "config", be.dscCfgPath)
			be.lastDSCWarnTime = now
		}
	}
	if os.IsNotExist(err) {
		return fmt.Errorf("DSC config dir is missing")
	}
	if fi != nil && !fi.IsDir() {
		return fmt.Errorf("found a file instead of a DSC config dir")
	}
	if err != nil {
		return fmt.Errorf("DSC config dir is inaccessible: %s", err)
	}
	return nil
}

func (be *Backend) writeDSCCfgFile(finalPath string, tgtEnv *backend.TargetEnv) error {
	transport := "tcp" // currently only NVMe/TCP is supported by the DSC BE

	f, err := os.CreateTemp(be.dscCfgPath, dscReservedPrefix)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %s", err)
	}
	tmpPath := f.Name()
	defer os.Remove(tmpPath) // should only work if the Rename() below failed

	var b strings.Builder
	for _, ep := range tgtEnv.DiscoveryEPs {
		_, err := fmt.Fprintf(&b, "-t %s -a %s -s %d -q %s -n %s\n",
			transport, ep.Host(), ep.Port(), be.hostNQN, tgtEnv.SubsysNQN)
		// builder's "Write always returns len(p), nil", but 'revive' insists...
		if err != nil {
			return fmt.Errorf("failed to format discovery EP string, of all things")
		}
	}

	if _, err = f.WriteString(b.String()); err != nil {
		return fmt.Errorf("failed to write temp file: %s", err)
	}
	if err = f.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %s", err)
	}

	err = os.Rename(tmpPath, finalPath)
	if err != nil {
		return fmt.Errorf("failed to rename temp file '%s' to '%s': %s",
			tmpPath, finalPath, err)
	}
	return nil
}

func (be *Backend) Attach(
	_ context.Context, tgtEnv *backend.TargetEnv, nguid guuid.UUID,
) *status.Status {
	if err := be.checkDSCCfgPath(); err != nil {
		return status.New(codes.Unknown, err.Error())
	}

	p := path.Join(be.dscCfgPath, nguid.String())
	be.log.Debug("creating DSC config file", "config", p)
	if err := be.writeDSCCfgFile(p, tgtEnv); err != nil {
		return status.Newf(codes.Unknown, "failed to create DSC config entries file: %s",
			err)
	}
	return nil
}

func (be *Backend) Detach(_ context.Context, nguid guuid.UUID) *status.Status {
	if err := be.checkDSCCfgPath(); err != nil {
		return status.New(codes.Unknown, err.Error())
	}

	// TODO: CSI spec mandates that it is effectively CO's sacred duty to
	// do the refcounting on volumes (which is facilitated by mandating
	// idempotent semantics of the plugins). unfortunately, COs are totally
	// oblivious of the LB "all volumes grow off the same subsystem, and
	// connect/disconnect must be done per subsystem, NOT volume" semantics.
	// which means we need to devise a way of detecting when and how to
	// disconnect the subsystem, in the face of:
	// 1. CSI plugin being ephemeral and with no persistent state - that's
	//    a bit hard to track, and:
	// 2. watch out for the races: the whole connect/disconnect machinery
	//    will need to be protected by a lock, because even if we trust
	//    k8s not to run more than one instance of this plugin in the same
	//    role on the same node (do expect Node+Controller side by side,
	//    though), k8s is only too eager to invoke the gRPC calls
	//    concurrently. and, apparently, NVMe-oF does NOT track block device
	//    usage by mounts (which still sounds very odd to me!!), so in a
	//    race you can easily end up disconnecting a volume that just got
	//    mounted - with a corresponding data loss (tried it, works).
	//  so i foresee some GetDeviceNameFromMount() under lock (that func
	//  and its ilk in k8s.io-land have odd notion of TOCTTOU) around here...
	be.log.Debug("NOT disconnecting from from the target to avoid races!")

	p := path.Join(be.dscCfgPath, nguid.String())
	be.log.Debug("deleting DSC config file", "config", p)
	err := os.Remove(p)
	if err != nil && !os.IsNotExist(err) {
		return status.Newf(codes.Unknown,
			"failed to delete DSC config entries file '%s': %s", p, err)
	}
	return nil
}
