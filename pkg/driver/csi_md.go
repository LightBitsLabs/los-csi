package driver

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/container-storage-interface/spec/lib/go/csi"
	guuid "github.com/google/uuid"
	"github.com/lightbitslabs/lb-csi/pkg/util/endpoint"
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
	volParRoot        = "parameters"
	volParMgmtEPKey   = "mgmt-endpoint"
	volParRepCntKey   = "replica-count"
	volParCompressKey = "compression"
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
// and may optionally include (if omitted - the default is "disabled"):
//     compression: <"enabled"|"disabled">
// e.g.:
//     mgmt-endpoint: 10.0.0.100:80,10.0.0.101:80
//     replica-count: 2
//     compression: enabled
type lbCreateVolumeParams struct {
	mgmtEPs      endpoint.Slice // LightOS mgmt API server endpoints.
	replicaCount uint32         // total number of volume replicas.
	compression  bool           // whether compression is enabled.
}

// ParseCSICreateVolumeParams parses the `parameters` K:V map passed to
// CreateVolume() and validates the contents. the returned lbCreateVolumeParams
// is only valid if the returned error is 'nil'.
func ParseCSICreateVolumeParams(params map[string]string) (lbCreateVolumeParams, error) {
	res := lbCreateVolumeParams{}
	volParKey := func(key string) string { return volParRoot + "." + key }
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

	return res, nil
}

// lbVolumeID: ---------------------------------------------------------------

// volIDRegex is used for initial syntactic validation of lbVolumeID
// as serialised into a string.
var volIDRegex *regexp.Regexp

func init() {
	volIDRegex = regexp.MustCompile(
		`^mgmt:([^|]+)\|` +
			`nguid:([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$`)
}

// lbVolumeID represents the contents of the `volume_id` field
// (`CreateVolumeResponse.volume.volume_id`) returned by the plugin to the CO
// from CreateVolume(), and then passed back to the plugin in most of the
// other CSI API calls. it contains the vital information required by the
// plugin in order to connect to a remote LB and manage volumes as per CO
// requests. in case of K8s, this field is also known as `volumeHandle`.
//
// for transmission on the wire, it's serialised into a string with the
// following fixed format:
//   mgmt:<host>:<port>[,<host>:<port>...]|nguid:<nguid>
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
// e.g.:
//   mgmt:10.0.0.1:80,10.0.0.2:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66
//   mgmt:lb01.net:80|nguid:6bb32fb5-99aa-4a4c-a4e7-30b7787bbd66
//
// TODO: the CSI spec mandates that strings "SHALL NOT" exceed 128 bytes.
// K8s is more lenient (at least 253 bytes, likely more). in any case, with
// the current `volume_id` format, at most 4 mgmt API server endpoints can
// be guaranteed to be supported. anything beyond that is at the mercy of
// the CO implementors (and user network admins assigning IP ranges)...
type lbVolumeID struct {
	mgmtEPs endpoint.Slice // LightOS mgmt API server endpoints.
	uuid    guuid.UUID     // NVMe "Identify NS Data Structure".
}

// String generates the string representation of lbVolumeID that will be
// passed back and forth between the CO and this plugin.
func (vid lbVolumeID) String() string {
	res := fmt.Sprintf("mgmt:%s|nguid:%s", vid.mgmtEPs, vid.uuid)
	return res
}

// ParseCSIVolumeID parses CSI wire-protocol-level `volume_id` string into its
// constituents and syntactically validates it. the returned lbVolumeID is
// only valid if the returned error is 'nil'.
func ParseCSIVolumeID(id string) (lbVolumeID, error) {
	vid := lbVolumeID{}

	if id == "" {
		return vid, fmt.Errorf("unspecified or empty")
	}

	idStr := volIDRegex.FindStringSubmatch(id)
	if len(idStr) != 3 {
		return vid, fmt.Errorf("'%s' is malformed", id)
	}
	var err error
	vid.mgmtEPs, err = endpoint.ParseCSV(idStr[1])
	if err != nil {
		return vid, fmt.Errorf("'%s' has invalid mgmt hunk: %s", id, err)
	}

	vid.uuid, err = guuid.Parse(idStr[2])
	if err != nil {
		return vid, fmt.Errorf("'%s' has invalid NGUID: %s", id, err)
	} else if vid.uuid == guuid.Nil {
		return vid, fmt.Errorf("'%s' has invalid nil NGUID", id)
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
		// TODO: currently we only support 'ext4'. additional FSes may require
		// mount opts validation, etc., so will likely require a bit of
		// scaffolding (TBD)... not to mention packaging the utils!
		if mntCap.FsType != "" && mntCap.FsType != "ext4" {
			return mkEinvalf("volume_capability.mount.fs_type",
				"unsupported FS: %s", mntCap.FsType)
		}

		// TODO: add support for custom mount flags
		if len(mntCap.MountFlags) > 0 {
			return mkEinval("volume_capability.mount.mount_flags",
				"custom mount flags are not supported")
		}
	case *csi.VolumeCapability_Block:
		// TODO: consider adding raw block volume support. till then...
		return mkEinval("volume_capability.block",
			"raw block volumes are not supported")
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
	switch volCap := accessType.(type) {
	case *csi.VolumeCapability_Mount:
		mntCap := volCap.Mount
		if mntCap.FsType == "ext4" {
			return true
		}
		return false
	case *csi.VolumeCapability_Block:
	default:
		return false
	}
	return false
}
