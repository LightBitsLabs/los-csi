// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// Copyright (C) 2020 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/container-storage-interface/spec/lib/go/csi"
	guuid "github.com/google/uuid"

	"github.com/lightbitslabs/los-csi/pkg/util/endpoint"
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

const (
	volPathField     = "volume_path"
	volIDField       = "volume_id"
	snapIDField      = "snapshot_id"
	capRangeField    = "capacity_range"
	capRangeLimField = capRangeField + ".limit_bytes"
	nodeIDField      = "node_id"
)

// lbCreateVolumeParams: -----------------------------------------------------

const (
	volParRoot          = "parameters"
	volParMgmtEPKey     = "mgmt-endpoint"
	volParRepCntKey     = "replica-count"
	volParCompressKey   = "compression"
	volParProjNameKey   = "project-name"
	volParMgmtSchemeKey = "mgmt-scheme"
)

var projNameRegex *regexp.Regexp

func init() {
	projNameRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,61}[a-z0-9])?$`)
}

// checkProjectName() checks syntactic validity of the LB "project" name, as
// specified in the request params of the outward-facing CSI API entrypoints.
// it shall be the single point of truth about the project name validity
// inside the LB CSI plugin as a whole.
func checkProjectName(field, proj string) error {
	if proj == "" {
		return mkEinvalMissing(field)
	}
	if !projNameRegex.MatchString(proj) {
		return mkEinvalf(field, "'%s'", proj)
	}
	return nil
}

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
//     mgmt-scheme: "grpcs"
//     project-name: <project-name>
//     replica-count: <num-replicas>
// may optionally include (if omitted - the default is "disabled"):
//     compression: <"enabled"|"disabled">
// e.g.:
//     mgmt-endpoint: 10.0.0.100:80,10.0.0.101:80
//     mgmt-scheme: grpcs
//     project-name: proj-3
//     replica-count: 2
//     compression: enabled
type lbCreateVolumeParams struct {
	mgmtEPs      endpoint.Slice // LightOS mgmt API server endpoints.
	replicaCount uint32         // total number of volume replicas.
	compression  bool           // whether compression is enabled.
	projectName  string         // project name.
	mgmtScheme   string         // currently must be 'grpcs'
}

func volParKey(key string) string {
	return volParRoot + "." + key
}

// parseCSICreateVolumeParams parses the `parameters` K:V map passed to
// CreateVolume() and validates the contents. the returned lbCreateVolumeParams
// is only valid if the returned error is 'nil'.
func parseCSICreateVolumeParams(params map[string]string) (lbCreateVolumeParams, error) {
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

	// optional field, originally defaulting to empty.
	//
	// TODO: project names were only optional during the transition period
	// and have been MANDATORY for a long time now. make it so!
	key = volParKey(volParProjNameKey)
	if proj, ok := params[volParProjNameKey]; ok {
		err = checkProjectName(key, proj)
		if err != nil {
			return res, err
		}
		res.projectName = proj
	}

	// optional field, defaulting to 'grpcs'.
	//
	// TODO: the option of NOT using 'grpcs' was only viable during the transition
	// period and 'grpcs' became MANDATORY a long time ago, so the code below
	// should be updated to only accept 'grpcs' as a valid value - if it was
	// specified at all - for reverse compatibility.
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

	return res, nil
}

// lbResourceID: ---------------------------------------------------------------

// resIDRegex is used for initial syntactic validation of `lbResourceID`
// string form: LB CSI plugin generated resource IDs that the COs use to
// uniquely identify volumes, snapshots, etc.
var resIDRegex *regexp.Regexp

func init() {
	//nolint:lll
	resIDRegex = regexp.MustCompile(
		`^mgmt:([^|]+)\|` +
			`nguid:([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})` +
			`(\|proj:([^[:cntrl:]| ]+))?` + // proj name syntax checked separately
			`(\|scheme:(grpc|grpcs))?$`)
}

// lbResourceID uniquely identifies a lightbits resource such as a volume / snapshot / etc.
// It is what the plugin returns to the CO in response to creating resources, then
// passed back to the plugin in most of the other CSI API calls.
// it contains the vital information required by the plugin in order to connect to a remote
// LB and manage resources as per CO requests.
//
// for transmission on the wire, it's serialised into a string with the
// following fixed format:
//   mgmt:<host>:<port>[,<host>:<port>...]|nguid:<nguid>[|proj:<proj>][|scheme:<scheme>]
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
//    <proj>    - project/tenant name on the LB cluster. this field is
//            temporarily optional in resource IDs for reverse compatibility,
//            though modern LB clusters will refuse requests without it.
//            see notes below in parseCSIResourceID().
//    <scheme>  - transport scheme for communicating with the LB cluster.
//            the only valid value is currently 'grpcs' (gRPC over TLS), and
//            scheme is temporarily optional, in which case it defaults to...
//            'grpcs'! modern LB clusters will refuse plain unencrypted gRPC
//            requests anyway. see below in parseCSIResourceID().
// e.g.:
//   mgmt:10.0.0.1:80,10.0.0.2:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:a|scheme:grpcs
//   mgmt:lb01.net:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66|proj:b|scheme:grpcs
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
	scheme   string // currently must be 'grpcs'
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

// parseCSIResourceID parses CSI wire-protocol-level `volume_id` string into its
// constituents and syntactically validates it. the returned lbResourceID is
// only valid if the returned error is 'nil'.
func parseCSIResourceID(id string) (lbResourceID, error) {
	vid := lbResourceID{}

	if id == "" {
		return vid, fmt.Errorf("unspecified or empty")
	}

	match := resIDRegex.FindStringSubmatch(id)
	if len(match) < 2 {
		return vid, fmt.Errorf("'%s' is malformed", id)
	}
	var err error
	vid.mgmtEPs, err = endpoint.ParseCSV(match[1])
	if err != nil {
		return vid, fmt.Errorf("'%s' has invalid mgmt endpoints list: %s", id, err)
	}

	vid.uuid, err = guuid.Parse(match[2])
	if err != nil {
		return vid, fmt.Errorf("'%s' has invalid NGUID: %s", id, err)
	} else if vid.uuid == guuid.Nil {
		return vid, fmt.Errorf("'%s' has invalid nil NGUID", id)
	}

	// optional field, originally defaulting to empty.
	//
	// TODO: this was only optional during the transition period and has been
	// MANDATORY for a long time. make it so!
	vid.projName = match[4]
	if vid.projName != "" {
		err = checkProjectName("", vid.projName)
		if err != nil {
			return vid, fmt.Errorf("'%s' has invalid project name: '%s'", id, vid.projName)
		}
	}

	// optional field, defaulting to 'grpcs'.
	//
	// TODO: the option of NOT using 'grpcs' was only viable during the transition
	// period and 'grpcs' became MANDATORY a long time ago, so:
	// 1. the regex should be updated to only accept 'grpcs' as a valid value for
	//    reverse compatibility?
	// 2. lbResourceID formatter should probably stop generating this field.
	vid.scheme = match[6]
	if vid.scheme == "" {
		vid.scheme = "grpcs"
	}

	return vid, nil
}

func parseCSIResourceIDEinval(field, id string) (lbResourceID, error) {
	if id == "" {
		return lbResourceID{}, mkEinvalMissing(field)
	}
	rid, err := parseCSIResourceID(id)
	if err != nil {
		return lbResourceID{}, mkEinval(field, err.Error())
	}
	return rid, nil
}

func parseCSIResourceIDEnoent(field, id string) (lbResourceID, error) {
	if id == "" {
		return lbResourceID{}, mkEinvalMissing(field)
	}
	rid, err := parseCSIResourceID(id)
	if err != nil {
		return lbResourceID{}, mkEnoent("bad value of '%s': %s", field, err)
	}
	return rid, nil
}

// CSI volume capabilities helpers: ------------------------------------------

var supportedAccessModes = []csi.VolumeCapability_AccessMode_Mode{
	csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
}

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
