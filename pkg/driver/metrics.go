package driver

import (
	"fmt"

	"golang.org/x/sys/unix"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Metrics represents the used and available bytes of the Volume.
type Metrics struct {
	// The time at which these stats were updated.
	Time metav1.Time

	// Used represents the total bytes used by the Volume.
	// Note: For block devices this maybe more than the total size of the files.
	Used int64

	// Capacity represents the total capacity (bytes) of the volume's
	// underlying storage. For Volumes that share a filesystem with the host
	// (e.g. emptydir, hostpath) this is the size of the underlying storage,
	// and will not equal Used + Available as the fs is shared.
	Capacity int64

	// Available represents the storage space available (bytes) for the
	// Volume. For Volumes that share a filesystem with the host (e.g.
	// emptydir, hostpath), this is the available space on the underlying
	// storage, and is shared with host processes and other Volumes.
	Available int64

	// InodesUsed represents the total inodes used by the Volume.
	InodesUsed int64

	// Inodes represents the total number of inodes available in the volume.
	// For volumes that share a filesystem with the host (e.g. emptydir, hostpath),
	// this is the inodes available in the underlying storage,
	// and will not equal InodesUsed + InodesFree as the fs is shared.
	Inodes int64

	// InodesFree represent the inodes available for the volume.  For Volumes that share
	// a filesystem with the host (e.g. emptydir, hostpath), this is the free inodes
	// on the underlying storage, and is shared with host processes and other volumes
	InodesFree int64

	// Normal volumes are available for use and operating optimally.
	// An abnormal volume does not meet these criteria.
	// This field is OPTIONAL. Only some csi drivers which support NodeServiceCapability_RPC_VOLUME_CONDITION
	// need to fill it.
	Abnormal *bool

	// The message describing the condition of the volume.
	// This field is OPTIONAL. Only some csi drivers which support capability_RPC_VOLUME_CONDITION
	// need to fill it.
	Message *string
}

type MetricsProvider interface {
	// GetMetrics returns the Metrics for the Volume. Maybe expensive for
	// some implementations.
	GetMetrics() (*Metrics, error)
}

var _ MetricsProvider = &metricsStatFS{}

// metricsStatFS represents a MetricsProvider that calculates the used and available
// Volume space by stat'ing and gathering filesystem info for the Volume path.
type metricsStatFS struct {
	// the directory path the volume is mounted to.
	path string
}

// newMetricsStatfs creates a new metricsStatFS with the Volume path.
func newMetricsStatFS(path string) MetricsProvider {
	return &metricsStatFS{path}
}

// See MetricsProvider.GetMetrics
// GetMetrics calculates the volume usage and device free space by executing "du"
// and gathering filesystem info for the Volume path.
func (md *metricsStatFS) GetMetrics() (*Metrics, error) {
	metrics := &Metrics{Time: metav1.Now()}
	if md.path == "" {
		return metrics, fmt.Errorf("path is empty")
	}

	err := md.getFsInfo(metrics)
	if err != nil {
		return metrics, err
	}

	return metrics, nil
}

// getFsInfo writes metrics.Capacity, metrics.Used and metrics.Available from the filesystem info
func (md *metricsStatFS) getFsInfo(metrics *Metrics) error {
	available, capacity, usage, inodes, inodesFree, inodesUsed, err := info(md.path)
	if err != nil {
		return err
	}
	metrics.Available = available
	metrics.Capacity = capacity
	metrics.Used = usage
	metrics.Inodes = inodes
	metrics.InodesFree = inodesFree
	metrics.InodesUsed = inodesUsed
	return nil
}

// info linux returns (available bytes, byte capacity, byte usage, total inodes, inodes free, inode usage, error)
// for the filesystem that path resides upon.
func info(path string) (int64, int64, int64, int64, int64, int64, error) {
	statfs := &unix.Statfs_t{}
	err := unix.Statfs(path, statfs)
	if err != nil {
		return 0, 0, 0, 0, 0, 0, err
	}

	// Available is blocks available * fragment size
	available := int64(statfs.Bavail) * int64(statfs.Bsize)

	// Capacity is total block count * fragment size
	capacity := int64(statfs.Blocks) * int64(statfs.Bsize)

	// Usage is block being used * fragment size (aka block size).
	usage := (int64(statfs.Blocks) - int64(statfs.Bfree)) * int64(statfs.Bsize)

	inodes := int64(statfs.Files)
	inodesFree := int64(statfs.Ffree)
	inodesUsed := inodes - inodesFree

	return available, capacity, usage, inodes, inodesFree, inodesUsed, nil
}
