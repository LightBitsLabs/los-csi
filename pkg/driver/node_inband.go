// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/lightbitslabs/los-csi/pkg/lb"
)

// the discovery-client daemon config keys carrying the node DH-HMAC-CHAP
// secret pair. the DC reads this file once, at startup.
const (
	dcHostSecretKey = "dhChapSecret"
	dcCtrlSecretKey = "dhChapCtrlSecret"

	// DefaultDCConfigPath is where the DC looks for its daemon config; the
	// deployment shares this path between the plugin and the DC containers.
	DefaultDCConfigPath = "/etc/discovery-client/discovery-client.yaml"
)

func dcConfigPathFor(inBandAuth bool) string {
	if inBandAuth {
		return DefaultDCConfigPath
	}
	return ""
}

// renderDCAuthConfig merges the secret pair into an existing DC config,
// preserving any other configuration the file carries: the file may be owned
// by a host discovery-client package rather than by this plugin.
func renderDCAuthConfig(existing []byte, secrets lb.TrustedHostSecrets) []byte {
	var out []string
	for _, line := range strings.Split(string(existing), "\n") {
		key, _, found := strings.Cut(line, ":")
		key = strings.TrimSpace(key)
		if line == "" || (found && (key == dcHostSecretKey || key == dcCtrlSecretKey)) {
			continue
		}
		out = append(out, line)
	}
	out = append(out,
		fmt.Sprintf("%s: %q", dcHostSecretKey, secrets.HostSecret),
		fmt.Sprintf("%s: %q", dcCtrlSecretKey, secrets.TargetSecret))
	return []byte(strings.Join(out, "\n") + "\n")
}

// parseDCAuthConfig extracts the secret pair from a config file previously
// written by renderDCAuthConfig(). unknown lines are ignored, missing keys
// yield empty fields.
func parseDCAuthConfig(raw []byte) lb.TrustedHostSecrets {
	secrets := lb.TrustedHostSecrets{}
	for _, line := range strings.Split(string(raw), "\n") {
		key, val, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		val = strings.Trim(strings.TrimSpace(val), `"`)
		switch strings.TrimSpace(key) {
		case dcHostSecretKey:
			secrets.HostSecret = val
		case dcCtrlSecretKey:
			secrets.TargetSecret = val
		}
	}
	return secrets
}

func readDCAuthConfig(path string) (lb.TrustedHostSecrets, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return lb.TrustedHostSecrets{}, nil
	}
	if err != nil {
		return lb.TrustedHostSecrets{}, err
	}
	return parseDCAuthConfig(raw), nil
}

func writeDCAuthConfig(path string, secrets lb.TrustedHostSecrets) error {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".dc-config-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(renderDCAuthConfig(existing, secrets)); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// getOrCreateTrustedHost makes sure this node is registered as a trusted
// host on the LB cluster serving `vid`, tolerating concurrent registration.
func (d *Driver) getOrCreateTrustedHost(
	ctx context.Context, clnt lb.Client, vid lbResourceID,
) error {
	_, err := clnt.GetTrustedHost(ctx, d.nodeID, vid.projName)
	if err == nil {
		return nil
	}
	if !isStatusNotFound(err) {
		return err
	}
	_, err = clnt.CreateTrustedHost(ctx, d.nodeID, vid.projName, d.hostNQN)
	if err != nil && !isStatusAlreadyExists(err) {
		return err
	}
	return nil
}

// ensureInBandAuth makes NVMe in-band (DH-HMAC-CHAP) authentication work for
// this node against the LB cluster serving `vid`, when that cluster has
// in-band auth enabled:
//
//   - the node is registered as a trusted host, and its secret pair is set
//     on the cluster (LB auto-generates the pair on first registration);
//   - the discovery-client daemon config at `d.dcConfigPath` carries the
//     pair, so the connections the DC initiates authenticate.
//
// the node holds ONE secret pair, shared by every cluster it attaches to:
// the DC applies its config to all connections, so a second cluster gets the
// pair adopted from the config file rather than a fresh auto-generated one.
// on a cluster without in-band auth this is a no-op, keeping legacy behavior.
func (d *Driver) ensureInBandAuth(
	ctx context.Context, log *logrus.Entry, clnt lb.Client, ci *lb.ClusterInfo, vid lbResourceID,
) error {
	if !ci.InBandAuthEnabled {
		return nil
	}
	if d.dcConfigPath == "" {
		return mkPrecond("LB cluster %s requires NVMe in-band auth, but the "+
			"plugin runs without in-band auth support, see --inband-auth", ci.UUID)
	}
	log = log.WithField("trusted-host", d.nodeID)

	d.ibaMtx.Lock()
	defer d.ibaMtx.Unlock()

	local, err := readDCAuthConfig(d.dcConfigPath)
	if err != nil {
		return mkEExec("failed to read DC config '%s': %s", d.dcConfigPath, err)
	}

	if err := d.getOrCreateTrustedHost(ctx, clnt, vid); err != nil {
		return mungeLBErr(log, err, "failed to register node as trusted host on LB "+
			"cluster %s", ci.UUID)
	}

	remote, err := clnt.GetTrustedHostSecrets(ctx, d.nodeID, vid.projName)
	if err != nil {
		return mungeLBErr(log, err, "failed to get trusted host secrets from LB "+
			"cluster %s", ci.UUID)
	}

	switch {
	case local.IsComplete() && *remote == local:
		return nil
	case local.IsComplete():
		// the node pair exists (minted by another cluster or an earlier
		// registration): push it, the DC can only present one pair.
		err = clnt.SetTrustedHostSecrets(ctx, d.nodeID, vid.projName, local)
		if err != nil {
			return mungeLBErr(log, err, "failed to set trusted host secrets on LB "+
				"cluster %s", ci.UUID)
		}
		return nil
	case remote.IsComplete():
		// no local pair (fresh pod, wiped config volume): adopt the pair
		// this cluster already holds for the node.
		return d.persistDCAuthConfig(log, *remote)
	default:
		err = clnt.SetTrustedHostSecrets(ctx, d.nodeID, vid.projName, lb.TrustedHostSecrets{})
		if err != nil {
			return mungeLBErr(log, err, "failed to auto-generate trusted host "+
				"secrets on LB cluster %s", ci.UUID)
		}
		remote, err = clnt.GetTrustedHostSecrets(ctx, d.nodeID, vid.projName)
		if err != nil {
			return mungeLBErr(log, err, "failed to get auto-generated trusted host "+
				"secrets from LB cluster %s", ci.UUID)
		}
		if !remote.IsComplete() {
			return mkEagain("LB cluster %s returned an incomplete trusted host "+
				"secret pair", ci.UUID)
		}
		return d.persistDCAuthConfig(log, *remote)
	}
}

func (d *Driver) persistDCAuthConfig(log *logrus.Entry, secrets lb.TrustedHostSecrets) error {
	if err := writeDCAuthConfig(d.dcConfigPath, secrets); err != nil {
		return mkEExec("failed to write DC config '%s': %s", d.dcConfigPath, err)
	}
	log.Infof("wrote node DH-HMAC-CHAP secret pair to DC config '%s'", d.dcConfigPath)
	return nil
}
