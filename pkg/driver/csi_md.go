// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// Copyright (C) 2020 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	guuid "github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/lightbitslabs/los-csi/pkg/util/endpoint"
)

var (
	ErrNotSpecifiedOrEmpty = errors.New("unspecified or empty")
	ErrMalformed           = errors.New("malformed")
)

// this file holds the definitions of - and helper functions for handling of -
// the various bits metadata that gets shovelled through the CSI API boundary.
// that includes both the generic CSI metadata and the LB-specific parts.
//
// the latter are passed by the plugin to the CO through the various impl-
// specific CSI API entrypoint response fields and then back from the CO to the
// plugin through the various impl-specific CSI API entrypoint request fields.
// all of this is completely opaque to the CO itself, which just takes care to
// accept these values (typically strings or string maps, as appropriate),
// preserve them between the API entrypoint invocations (typically attached to
// the relevant CO-specific entities, e.g. SC or PV in K8s), and pass them back
// to the plugin in a fashion defined by the CSI spec.

// lbCreateVolumeParams: -----------------------------------------------------

const (
	volParRoot          = "parameters"
	volParMgmtEPKey     = "mgmt-endpoint"
	volParRepCntKey     = "replica-count"
	volParCompressKey   = "compression"
	volParProjNameKey   = "project-name"
	volParMgmtSchemeKey = "mgmt-scheme"

	// volEncryptedKey parameter in the storageclass parameter, can be either enabled|disabled
	volEncryptedKey = "encryption"
	// volEncryptionPassphraseKey name of the secret for the encryption passphrase
	volEncryptionPassphraseKey = "encryptionPassphrase"
	// volEncryptionPassphraseKeyMaxLen defines the maximum len of the encryption passphrase
	// this is according to the cryptsetup man page
	volEncryptionPassphraseKeyMaxLen = 512
)

// lbCreateVolumeParams represents the contents of the `parameters` field
// (`CreateVolumeRequest.parameters`) passed to the plugin by the CO on
// CreateVolume() CSI API entrypoint invocation. this supplementary info
// is used by the plugin to locate the relevant LightOS management API servers
// to connect to and to request that the volume be created with the specified
// LightOS-specific properties. the initial source of the these `parameters`
// is CO-specific (e.g. in K8s they're taken from the SC `parameters` stanza).
//
// `parameters` as passed to CreateVolume() is a string-to-string (!) KV map
// that must include:
//     mgmt-endpoint: <host>:<port>[,<host>:port>...]
//     replica-count: <num-replicas>
// may optionally include (if omitted - the default is "disabled"):
//     compression: <"enabled"|"disabled">
// and may optionally include (if omitted - the default is empty string - ""):
//     project-name: <valid-project-name>
// e.g.:
//     mgmt-endpoint: 10.0.0.100:80,10.0.0.101:80
//     replica-count: 2
//     compression: enabled
//     project-name: proj-3
type lbCreateVolumeParams struct {
	mgmtEPs      endpoint.Slice // LightOS mgmt API server endpoints.
	replicaCount uint32         // total number of volume replicas.
	compression  bool           // whether compression is enabled.
	projectName  string         // project name.
	mgmtScheme   string         // one of [grpc, grpcs]
	encrypted    bool           // if set to true, volume must be encrypted
}

func volParKey(key string) string {
	return volParRoot + "." + key
}

// ParseCSICreateVolumeParams parses the `parameters` K:V map passed to
// CreateVolume() and validates the contents. the returned lbCreateVolumeParams
// is only valid if the returned error is 'nil'.
func ParseCSICreateVolumeParams(params map[string]string) (lbCreateVolumeParams, error) {
	res := lbCreateVolumeParams{}
	var err error

	key := volParKey(volParMgmtEPKey)
	mgmtEPs := params[volParMgmtEPKey]
	if mgmtEPs == "" {
		return res, mkEinvalMissing(key)
	}
	res.mgmtEPs, err = endpoint.ParseCSV(mgmtEPs)
	if err != nil {
		return res, mkEinval(key, err.Error())
	}

	key = volParKey(volParRepCntKey)
	replicaCount := params[volParRepCntKey]
	if replicaCount == "" {
		return res, mkEinvalMissing(key)
	}
	repCnt, err := strconv.ParseUint(replicaCount, 10, 32)
	if err != nil {
		return res, mkEinvalf(key, "'%s'", replicaCount)
	}
	res.replicaCount = uint32(repCnt)

	key = volParKey(volParCompressKey)
	switch params[volParCompressKey] {
	case "", "disabled":
		res.compression = false
	case "enabled":
		res.compression = true
	default:
		return res, mkEinval(key, params[volParCompressKey])
	}

	key = volParKey(volParProjNameKey)
	if projectName, ok := params[volParProjNameKey]; !ok {
		res.projectName = ""
	} else {
		proj := projNameRegex.FindString(projectName)
		if len(proj) == 0 {
			return res, mkEinval(key, params[volParProjNameKey])
		}
		res.projectName = projectName
	}

	key = volParKey(volParMgmtSchemeKey)
	mgmtScheme := params[volParMgmtSchemeKey]
	switch mgmtScheme {
	case "", "grpcs":
		res.mgmtScheme = "grpcs"
	case "grpc":
		res.mgmtScheme = "grpc"
	default:
		return res, mkEinval(key, mgmtScheme)
	}

	isEncrypted, err := isVolumeEncryptionSet(params)
	if err != nil {
		return res, err
	}
	res.encrypted = isEncrypted

	return res, nil
}

func isVolumeEncryptionSet(params map[string]string) (bool, error) {
	if encryptedStringValue, ok := params[volEncryptedKey]; ok {
		switch encryptedStringValue {
		case "enabled", "true":
			return true, nil
		case "", "disabled", "false":
			return false, nil
		default:
			return false, status.Errorf(
				codes.InvalidArgument,
				"invalid value %q for parameter %q, only enabled|disabled|true|false allowed",
				encryptedStringValue, volEncryptedKey)
		}
	}
	return false, nil
}

// lbResourceID: ---------------------------------------------------------------

// resIDRegex is used for initial syntactic validation of LB resource IDS (volumes, snapshots, etc.)
// as serialised into a string.
var resIDRegex *regexp.Regexp
var projNameRegex *regexp.Regexp

func init() {
	projNameRegex = regexp.MustCompile(`^[a-z0-9-\.]{1,63}$`)

	resIDRegex = regexp.MustCompile(
		`^mgmt:(?P<ep>[^|]+)\|` +
			`nguid:(?P<nguid>[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})` +
			`(\|(?P<proj>proj:[a-z0-9-\.]{1,63}))?` +
			`(\|(?P<scheme>scheme:(grpc|grpcs)))?$`)
}

// lbResourceID uniquely identifies a lightbits resource such as a volume / snapshot / etc.
// It is what the plugin returns to the CO in response to creating resources, then
// passed back to the plugin in most of the other CSI API calls.
// it contains the vital information required by the plugin in order to connect to a remote
// LB and manage resources as per CO requests.
//
// for transmission on the wire, it's serialised into a string with the
// following fixed format:
//   mgmt:<host>:<port>[,<host>:<port>...]|nguid:<nguid>|proj:<proj>||scheme:<grpc|grpcs>
// where:
//    <host>    - mgmt API server endpoint of the LightOS cluster hosting the
//            volume. can be a hostname or an IP address. more than one
//            comma-separated <host>:<port> pair can be specified.
//            TODO: IPv6 support will require amending parsing for bizarre
//            extra allowed characters.
//    <port>    - variable-length printable decimal representation of the
//            uint16 port number, no leading zeroes.
//    <nguid>   - volume NGUID (see NVMe spec, Identify NS Data Structure)
//            in its "canonical", 36-character long, RFC-4122 compliant string
//            representation.
//    <proj>    - project name on LightOS cluster. this field is OPTIONAL, in
//			  which case the cluster is configured with multi-tenancy disabled.
//    <scheme> - project name on LightOS cluster. this field is OPTIONAL, in
//			  which case the scheme is set to grpcs.
// e.g.:
//   mgmt:10.0.0.1:80,10.0.0.2:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66
//   mgmt:lb01.net:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66
//   mgmt:lb01.net:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:p1
//   mgmt:lb01.net:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:project-4
//   mgmt:lb01.net:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:project-4|scheme:grpc
//   mgmt:lb01.net:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|scheme:grpcs
//
// TODO: the CSI spec mandates that strings "SHALL NOT" exceed 128 bytes.
// K8s is more lenient (at least 253 bytes, likely more). in any case, with
// the current `volume_id` format, at most 4 mgmt API server endpoints can
// be guaranteed to be supported. anything beyond that is at the mercy of
// the CO implementors (and user network admins assigning IP ranges)...
type lbResourceID struct {
	mgmtEPs  endpoint.Slice // LightOS mgmt API server endpoints.
	uuid     guuid.UUID     // NVMe "Identify NS Data Structure".
	projName string
	scheme   string // one of [grpc, grpcs]
}

// String generates the string representation of lbResourceID that will be
// passed back and forth between the CO and this plugin.
func (vid lbResourceID) String() string {
	res := fmt.Sprintf("mgmt:%s|nguid:%s", vid.mgmtEPs, vid.uuid)
	if len(vid.projName) > 0 {
		res += fmt.Sprintf("|proj:%s", vid.projName)
	}
	if len(vid.scheme) > 0 {
		res += fmt.Sprintf("|scheme:%s", vid.scheme)
	}
	return res
}

// ParseCSIResourceID parses CSI wire-protocol-level `volume_id` string into its
// constituents and syntactically validates it. the returned lbResourceID is
// only valid if the returned error is 'nil'.
func ParseCSIResourceID(id string) (lbResourceID, error) {
	vid := lbResourceID{}

	if id == "" {
		return vid, ErrNotSpecifiedOrEmpty
	}

	match := resIDRegex.FindStringSubmatch(id)
	if len(match) < 2 {
		return vid, fmt.Errorf("bad volume id: '%s'. must contain at least 2 items. err: %w", id, ErrMalformed)
	}
	result := make(map[string]string)
	for i, name := range resIDRegex.SubexpNames() {
		if i != 0 && name != "" {
			result[name] = match[i]
		}
	}
	var err error
	if ep, ok := result["ep"]; ok {
		vid.mgmtEPs, err = endpoint.ParseCSV(ep)
		if err != nil {
			return vid, fmt.Errorf("'%s' has invalid mgmt hunk: %s", id, err)
		}
	} else {
		return vid, fmt.Errorf("bad volume id: '%s'. missing mgmt-ep part. err: %w", id, ErrMalformed)
	}

	if ep, ok := result["nguid"]; ok {
		vid.uuid, err = guuid.Parse(ep)
		if err != nil {
			return vid, fmt.Errorf("'%s' has invalid NGUID: %s", id, err)
		} else if vid.uuid == guuid.Nil {
			return vid, fmt.Errorf("'%s' has invalid nil NGUID", id)
		}
	} else {
		return vid, fmt.Errorf("bad volume id: '%s'. missing nguid part. err: %w", id, ErrMalformed)
	}

	if proj, ok := result["proj"]; ok {
		// optional field
		if proj != "" {
			splitted := strings.Split(proj, ":")
			if len(splitted) != 2 {
				return vid, fmt.Errorf("'%s' invalid projName", proj)
			}
			vid.projName = splitted[1]
		}
	}

	if scheme, ok := result["scheme"]; ok {
		// optional field
		if scheme != "" {
			splitted := strings.Split(scheme, ":")
			if len(splitted) != 2 {
				return vid, fmt.Errorf("'%s' invalid scheme", scheme)
			}
			vid.scheme = splitted[1]
		} else {
			vid.scheme = "grpcs"
		}
	}

	return vid, nil
}

// CSI volume capabilities helpers: ------------------------------------------

// see validateVolumeCapability() docs for info.
func (d *Driver) validateVolumeCapabilities(caps []*csi.VolumeCapability) error {
	if len(caps) == 0 {
		return mkEinvalMissing("volume_capability")
	}
	for _, c := range caps {
		if err := d.validateVolumeCapability(c); err != nil {
			return err
		}
	}
	return nil
}

// performs a generic driver-level capability validation to weed out
// capabilities that are unsupported for sure. specific volumes might have
// additional constraints once they're created, which need to be validated
// separately.
func (d *Driver) validateVolumeCapability(c *csi.VolumeCapability) error {
	if c == nil {
		return mkEinvalMissing("volume_capability")
	}

	modeOk := false
	accessMode := c.GetAccessMode()
	if accessMode == nil {
		return mkEinvalMissing("volume_capability.access_mode")
	}
	for _, m := range supportedAccessModes {
		if m == accessMode.Mode {
			modeOk = true
			break
		}
	}
	if !modeOk {
		return mkEinvalf("volume_capability.access_mode",
			"unsupported mode: %d", accessMode.Mode)
	}

	accessType := c.GetAccessType()
	switch volCap := accessType.(type) {
	case *csi.VolumeCapability_Mount:
		mntCap := volCap.Mount
		// TODO: currently we only support 'ext4' and 'xfs'. additional FSes may require
		// mount opts validation, etc., so will likely require a bit of
		// scaffolding (TBD)... not to mention packaging the utils!
		if mntCap.FsType != "" && mntCap.FsType != "ext4" && mntCap.FsType != "xfs" {
			return mkEinvalf("volume_capability.mount.fs_type",
				"unsupported FS: %s", mntCap.FsType)
		}

		// TODO: add support for custom mount flags
		if len(mntCap.MountFlags) > 0 {
			return mkEinval("volume_capability.mount.mount_flags",
				"custom mount flags are not supported")
		}
	case *csi.VolumeCapability_Block:
	case nil:
		return mkEinvalMissing("volume_capability.access_type")
	default:
		return mkEinval("volume_capability.access_type",
			"unexpected access type specified")
	}

	return nil
}

func (d *Driver) nodeExpansionRequired(c *csi.VolumeCapability) bool {
	accessType := c.GetAccessType()
	switch accessType.(type) {
	case *csi.VolumeCapability_Mount:
		// for some reason mount.FsType can be empty at this stage so we will call expand
		// volume, and let the node do it's thing
		return true
	case *csi.VolumeCapability_Block:
	default:
		return false
	}
	return false
}
