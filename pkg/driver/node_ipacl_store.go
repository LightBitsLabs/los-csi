// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	guuid "github.com/google/uuid"
)

// ipACLStateDir keeps the IP-ACL re-check list across plugin restarts, after
// which the CO does not stage the volumes again. it is on tmpfs, so the JWTs
// never reach a disk: a node reboot clears it, and the CO then stages the
// volumes again.
const ipACLStateDir = "/run/lb-csi/ipacl"

const ipACLStateExt = ".json"

type ipACLVolState struct {
	VolumeHandle string `json:"volumeHandle"`
	JWT          string `json:"jwt"`
}

func (l *ipACLVolList) statePath(uuid guuid.UUID) string {
	return filepath.Join(l.dir, uuid.String()+ipACLStateExt)
}

func (l *ipACLVolList) save(vid lbResourceID, jwt string) error {
	if l.dir == "" {
		return nil
	}
	if err := os.MkdirAll(l.dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(ipACLVolState{VolumeHandle: vid.String(), JWT: jwt})
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(l.dir, ".tmp-*")
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), l.statePath(vid.uuid))
	}
	if err != nil {
		os.Remove(f.Name())
	}
	return err
}

func (l *ipACLVolList) remove(uuid guuid.UUID) error {
	if l.dir == "" {
		return nil
	}
	err := os.Remove(l.statePath(uuid))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// load tracks the saved volumes, and returns how many it tracked. it skips
// an unreadable entry and goes on with the others.
func (l *ipACLVolList) load() (int, error) {
	if l.dir == "" {
		return 0, nil
	}
	ents, err := os.ReadDir(l.dir)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	l.mtx.Lock()
	defer l.mtx.Unlock()
	if l.vols == nil {
		l.vols = map[guuid.UUID]*ipACLVol{}
	}
	var errs []error
	for _, e := range ents {
		path := filepath.Join(l.dir, e.Name())
		if filepath.Ext(path) != ipACLStateExt {
			// a save cut short by a restart.
			os.Remove(path)
			continue
		}
		vid, jwt, err := readIPAclVolState(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
			continue
		}
		l.vols[vid.uuid] = &ipACLVol{vid: vid, jwt: jwt}
	}
	return len(l.vols), errors.Join(errs...)
}

func readIPAclVolState(path string) (lbResourceID, string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return lbResourceID{}, "", err
	}
	var st ipACLVolState
	if err := json.Unmarshal(b, &st); err != nil {
		return lbResourceID{}, "", err
	}
	vid, err := parseCSIResourceID(st.VolumeHandle)
	if err != nil {
		return lbResourceID{}, "", err
	}
	if !vid.ipACL || filepath.Base(path) != vid.uuid.String()+ipACLStateExt {
		return lbResourceID{}, "", fmt.Errorf("volume ID '%s' does not match the file", st.VolumeHandle)
	}
	return vid, st.JWT, nil
}
