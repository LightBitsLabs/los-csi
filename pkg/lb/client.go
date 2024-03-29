// Copyright (C) 2016--2020 Lightbits Labs Ltd.
// SPDX-License-Identifier: Apache-2.0

package lb

import (
	"context"
	"fmt"
	"strings"
	"time"

	guuid "github.com/google/uuid"

	"github.com/lightbitslabs/los-csi/pkg/util/endpoint"
)

const (
	unknown = "<UNKNOWN>"
)

type VolumeState int32

// match present LB API values. here's to API stability!
const (
	VolumeStateUnknown VolumeState = 0
	VolumeCreating     VolumeState = 1
	VolumeAvailable    VolumeState = 2
	VolumeDeleting     VolumeState = 3

	// TODO: remove deprecated states once the LightOS API drops them:
	VolumeDeletedDEPRECATED VolumeState = 4

	VolumeFailed   VolumeState = 7
	VolumeUpdating VolumeState = 8

	VolumeRollback  VolumeState = 9
	VolumeMigrating VolumeState = 10
)

func (s VolumeState) String() string {
	switch s { //nolint:exhaustive
	case VolumeCreating:
		return "creating" //nolint:goconst // unrelated to the other one.
	case VolumeAvailable:
		return "available" //nolint:goconst // unrelated to the other one.
	case VolumeDeleting:
		return "deleting" //nolint:goconst // unrelated to the other one.
	case VolumeFailed:
		return "failed" //nolint:goconst // unrelated to the other one.
	case VolumeUpdating:
		return "updating"
	case VolumeRollback:
		return "rollback" //nolint:goconst // unrelated to the other one.
	case VolumeMigrating:
		return "migrating" //nolint:goconst // unrelated to the other one.

	// TODO: remove deprecated fields once the API drops them
	case VolumeDeletedDEPRECATED:
		return "deleted" //nolint:goconst // unrelated to the other one.
	}
	return unknown
}

type VolumeProtection int32

// match present LB API values. here's to API stability!
const (
	VolumeProtectionUnknown VolumeProtection = 0
	VolumeProtected         VolumeProtection = 1
	VolumeDegraded          VolumeProtection = 2
	VolumeReadOnly          VolumeProtection = 3
	VolumeNotAvailable      VolumeProtection = 4
)

func (s VolumeProtection) String() string {
	switch s { //nolint:exhaustive
	case VolumeProtected:
		return "fully-protected"
	case VolumeDegraded:
		return "degraded"
	case VolumeReadOnly:
		return "read-only"
	case VolumeNotAvailable:
		return "not-available"
	}
	return unknown
}

type Volume struct {
	// "core" volume properties. q.v. IsSameAs().
	Name               string
	UUID               guuid.UUID
	ReplicaCount       uint32
	Capacity           uint64
	LogicalUsedStorage uint64
	Compression        bool
	SnapshotUUID       guuid.UUID
	QosPolicyName      string

	ACL []string

	State      VolumeState
	Protection VolumeProtection

	ETag        string
	ProjectName string
}

func (v *Volume) IsAccessible() bool {
	return v.State == VolumeAvailable || v.State == VolumeUpdating || v.State == VolumeMigrating
}

func (v *Volume) IsWritable() bool {
	return v.Protection == VolumeProtected || v.Protection == VolumeDegraded
}

func (v *Volume) IsUsable() bool {
	return v.IsAccessible() && v.IsWritable()
}

// IsSameAs compares only "core" volume properties, rather than transient
// state, such as the current State, Protection and ACL.
func (v *Volume) IsSameAs(other *Volume) bool {
	return v.Name == other.Name &&
		v.UUID == other.UUID &&
		v.ReplicaCount == other.ReplicaCount &&
		v.Capacity == other.Capacity &&
		v.Compression == other.Compression
}

type excuses []string

func (e *excuses) and(format string, args ...interface{}) {
	*e = append(*e, fmt.Sprintf(format, args...))
}

const (
	SkipNone = 0
	SkipUUID = (1 << iota)
	SkipSnapUUID
	SkipCapacity
)

// ExplainDiffsFrom returns a list of human-readable sentences describing the
// differences between a pair of volumes. the volumes are referred to in the
// output as lDescr and rDescr, a pair of adjectives will work well for those.
// only "core" volume properties are examined.
func (v *Volume) ExplainDiffsFrom(other *Volume, lDescr, rDescr string, skipFields uint32) []string {
	not := func(field uint32) bool { return skipFields&field == 0 }

	if !strings.HasSuffix(lDescr, " ") {
		lDescr += " "
	}
	if rDescr == "" {
		rDescr = "other"
	}

	var diffs excuses
	if v.Name != other.Name {
		diffs.and("%svolume name '%s' differs from the %s volume name '%s'",
			lDescr, v.Name, rDescr, other.Name)
	}
	if not(SkipUUID) && v.UUID != other.UUID {
		diffs.and("%svolume UUID %s differs from the %s volume UUID %s",
			lDescr, v.UUID, rDescr, other.UUID)
	}
	if v.ReplicaCount != other.ReplicaCount {
		diffs.and("%sreplica count of %d differs from the %s replica count of %d",
			lDescr, v.ReplicaCount, rDescr, other.ReplicaCount)
	}
	if not(SkipCapacity) && v.Capacity != other.Capacity {
		diffs.and("rounded up %scapacity %dB differs from the %s volume capacity %dB",
			lDescr, v.Capacity, rDescr, other.Capacity)
	}
	if v.Compression != other.Compression {
		b2s := map[bool]string{false: "disabled", true: "enabled"}
		diffs.and("%scompression is %s while the %s volume has compression %s",
			lDescr, b2s[v.Compression], rDescr, b2s[other.Compression])
	}
	if v.ProjectName != other.ProjectName {
		diffs.and("%sVolume %s project name %q differs from the %s volume project name %q",
			lDescr, v.Name, v.ProjectName, rDescr, other.ProjectName)
	}
	if not(SkipSnapUUID) && v.SnapshotUUID != other.SnapshotUUID {
		diffs.and("%sVolume %s source snapshot %s differs from the %s volume source snapshot %s",
			lDescr, v.Name, v.SnapshotUUID, rDescr, other.SnapshotUUID)
	}

	return diffs
}

//revive:disable:var-naming // prefer to match the underlying LB API capitalisation
type ClusterInfo struct {
	UUID               guuid.UUID
	SubsysNQN          string
	CurrMaxReplicas    uint32
	MaxReplicas        uint32
	DiscoveryEndpoints []string
	ApiEndpoints       []string
	NvmeEndpoints      []string
}

type Cluster struct {
	UUID               guuid.UUID
	SubsysNQN          string
	CurrMaxReplicas    uint32
	MaxReplicas        uint32
	DiscoveryEndpoints []string
	ApiEndpoints       []string
	Capacity           uint64
}

//revive:enable:var-naming

type NodeState int32

// match present LB API values. here's to API stability!
const (
	NodeStateUnknown NodeState = 0
	NodeActive       NodeState = 1
	NodeActivating   NodeState = 2
	NodeInactive     NodeState = 3
	NodeUnattached   NodeState = 4
	NodeAttaching    NodeState = 6
	NodeDetaching    NodeState = 7
)

func (s NodeState) String() string {
	switch s { //nolint:exhaustive
	case NodeActive:
		return "active"
	case NodeActivating:
		return "activating"
	case NodeInactive:
		return "inactive"
	case NodeUnattached:
		return "unattached"
	case NodeAttaching:
		return "attaching"
	case NodeDetaching:
		return "detaching"
	}
	return unknown
}

type Node struct {
	Name     string
	UUID     guuid.UUID
	DataEP   endpoint.EP
	HostName string
	State    NodeState
}

// for sort.Interface:
type NodesByName []*Node

func (n NodesByName) Len() int           { return len(n) }
func (n NodesByName) Swap(i, j int)      { n[i], n[j] = n[j], n[i] }
func (n NodesByName) Less(i, j int) bool { return n[i].Name < n[j].Name }

const (
	// these two are supposedly mutually exclusive...
	ACLAllowAny  = "ALLOW_ANY"
	ACLAllowNone = "ALLOW_NONE"
)

// VolumeUpdate struct expresses the desired state of the specified (i.e.
// non-nil/default) volume properties.
type VolumeUpdate struct {
	// full desired target ACL. nil slice to not update ACL, empty slice
	// to clear ACL.
	ACL []string

	Capacity uint64
}

// VolumeUpdateHook is passed to Client.UpdateVolume(). it will be invoked
// by UpdateVolume() with the freshly updated state of the volume as `vol`
// param. if updates to the volume are required, VolumeUpdateHook should return
// `update` with the desired state of the specified fields, otherwise - nil
// `update` should be returned, causing the update operation to be skipped. if
// the UpdateVolume() operation should be aborted, VolumeUpdateHook should
// return non-nil `err`, this error will be propagated back verbatim to the
// UpdateVolume() caller. care should, therefore, be taken when choosing the
// error type and value to return, if the ability to differentiate between
// the local/remote client/server errors and VolumeUpdateHook errors is a
// consideration.
type VolumeUpdateHook func(vol *Volume) (update *VolumeUpdate, err error)

type SnapshotState int32

const (
	SnapshotStateUnknown SnapshotState = 0
	SnapshotCreating     SnapshotState = 1
	SnapshotAvailable    SnapshotState = 2
	SnapshotDeleting     SnapshotState = 3
	SnapshotDeleted      SnapshotState = 4
	SnapshotFailed       SnapshotState = 7
)

func (s SnapshotState) String() string {
	switch s { //nolint:exhaustive
	case SnapshotCreating:
		return "creating"
	case SnapshotAvailable:
		return "available"
	case SnapshotDeleting:
		return "deleting"
	case SnapshotDeleted:
		return "deleted"
	case SnapshotFailed:
		return "failed"
	}
	return unknown
}

type Snapshot struct {
	// "core" snapshot properties.
	Name               string
	UUID               guuid.UUID
	Capacity           uint64
	State              SnapshotState
	SrcVolUUID         guuid.UUID
	SrcVolName         string
	SrcVolReplicaCount uint32
	SrcVolCompression  bool
	CreationTime       time.Time

	ETag        string
	ProjectName string
}

//nolint:gofumpt
type Client interface {
	Close()
	// ID() returns a unique opaque string ID of this client.
	ID() string
	// Targets() returns a sorted list of unique target endpoints this instance handles.
	Targets() string

	RemoteOk(ctx context.Context) error
	GetCluster(ctx context.Context) (*Cluster, error)
	GetClusterInfo(ctx context.Context) (*ClusterInfo, error)
	ListNodes(ctx context.Context) ([]*Node, error)

	CreateVolume(ctx context.Context, name string, capacity uint64,
		replicaCount uint32, compress bool, acl []string, projectName string,
		snapshotID guuid.UUID, qosPolicyName string, blocking bool,
	) (*Volume, error)
	DeleteVolume(ctx context.Context, uuid guuid.UUID, projectName string, blocking bool) error
	GetVolume(ctx context.Context, uuid guuid.UUID, projectName string) (*Volume, error)
	GetVolumeByName(ctx context.Context, name string, projectName string) (*Volume, error)
	UpdateVolume(ctx context.Context, uuid guuid.UUID, projectName string,
		hook VolumeUpdateHook,
	) (*Volume, error)

	CreateSnapshot(ctx context.Context, name string, projectName string, srcVolUUID guuid.UUID,
		descr string, blocking bool,
	) (*Snapshot, error)
	DeleteSnapshot(ctx context.Context, uuid guuid.UUID, projectName string, blocking bool) error
	GetSnapshot(ctx context.Context, uuid guuid.UUID, projectName string) (*Snapshot, error)
	GetSnapshotByName(ctx context.Context, name string, projectName string) (*Snapshot, error)
}
