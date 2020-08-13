package driver

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/container-storage-interface/spec/lib/go/csi"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_logrus "github.com/grpc-ecosystem/go-grpc-middleware/logging/logrus"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	"github.com/lightbitslabs/lb-csi/pkg/grpcutil"
	"github.com/lightbitslabs/lb-csi/pkg/lb"
	"github.com/lightbitslabs/lb-csi/pkg/lb/lbgrpc"
	"github.com/lightbitslabs/lb-csi/pkg/util/endpoint"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/util/mount"
)

const (
	// q.v. https://github.com/container-storage-interface/spec/blob/v1.0.0/spec.md#getplugininfo
	driverName = "csi.lightbitslabs.com"

	logTimestampFmt = "2006-01-02T15:04:05.000000-07:00"
)

var (
	// SHOULD be inserted at build time through `-ldflags`
	version          = "0.0.0"
	versionGitCommit = ""
	versionBuildHash = ""
	versionBuildID   = ""

	supportedAccessModes = []csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
	}
	accessModesCache []*csi.VolumeCapability_AccessMode
)

func GetVersion() string {
	return version
}

func GetFullVersionStr() string {
	ver := fmt.Sprintf("%s (GitCommit: %s", version, versionGitCommit)
	if versionBuildHash != "" {
		ver += fmt.Sprintf(", BuildHash: %s", versionBuildHash)
	}
	if versionBuildID != "" {
		ver += fmt.Sprintf(", BuildID: %s", versionBuildID)
	}
	return ver + ")"
}

type Config struct {
	NodeID   string
	Endpoint string // must be a Unix Domain Socket URI

	DefaultFS string // one of: ext4

	LogLevel      string // one of: debug/info/warn/error
	LogRole       string
	LogTimestamps bool
	LogFormat     string

	// hidden, dev-only options:
	BinaryName    string
	Transport     string // one of: tcp/rdma
	SquelchPanics bool
	PrettyJson    bool
}

type Driver struct {
	sockPath  string // control UDS path
	nodeID    string
	hostNQN   string
	defaultFS string

	srv *grpc.Server
	log *logrus.Entry

	lbclients *lb.ClientPool

	mounter *mount.SafeFormatAndMount

	// only 'tcp' is properly supported, 'rdma' is a dev/test-only hack
	transport string

	// this should be used for dev/test only: if the driver tanked with a
	// panic, all bets on its internal state are off. the only reason to
	// run in this mode is to have these panics observable as errors at
	// the gRPC level so you can spot them from remote.
	squelchPanics bool

	// TODO: a gross hack, obviously. a more sensible thing to do would be
	// to have a set of named per-volume_id GLocks, and a set of per-NVMe-oF
	// connection-set (transport/addr/port/SubNQN/HostNQN) GLocks, locked
	// in that order. that would allow for concurrent local-only ops on
	// unrelated volumes and concurrent ops involving network on unrelated
	// targets. since quite a few of the ops the driver does might block
	// for significant amounts of time (think NVMe-oF network timeouts,
	// mgmt API timeouts, the time it takes to fsck a 4TB ext4, etc.), a
	// single huge lock is unfortunate, but safety first...
	bdl sync.Mutex
}

const (
	maxHostNQNLen = 223 // see section 7.9 of NVMe base spec
	hostNQNPrefix = "nqn.2019-09.com.lightbitslabs:host:"
	maxNodeIDLen  = maxHostNQNLen - len(hostNQNPrefix)
)

var nodeIDRegex *regexp.Regexp

func init() {
	nodeIDRegex = regexp.MustCompile(`^[a-zA-Z0-9.-]+$`)
}

func checkNodeID(nodeID string) error {
	if nodeID == "" {
		return fmt.Errorf("unspecified or empty")
	}
	if !nodeIDRegex.MatchString(nodeID) {
		return fmt.Errorf("invalid characters found, ID must contain only alphanumeric " +
			"characters, periods and hyphens")
	}
	n := len(nodeID)
	if n > maxNodeIDLen {
		return fmt.Errorf("%d bytes specified, limit is %d", n, maxNodeIDLen)
	}
	return nil
}

func nodeIDToHostNQN(nodeID string) string {
	return hostNQNPrefix + nodeID
}

// hostNQNToNodeID() attempts to extrapolate the original NodeID from the
// Host NQN (volume ACL) - the inverse of nodeIDToHostNQN(). does NOT actually
// validate the resultant NodeID. returns an empty string if it fails.
func hostNQNToNodeID(hostNQN string) string {
	if strings.HasPrefix(hostNQN, hostNQNPrefix) {
		return hostNQN[len(hostNQNPrefix):]
	}
	return ""
}

func New(cfg Config) (*Driver, error) {
	if err := checkNodeID(cfg.NodeID); err != nil {
		return nil, errors.Wrapf(err, "bad node ID '%s'", cfg.NodeID)
	}
	u, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, errors.Wrapf(err, "bad endpoint address '%s'", cfg.Endpoint)
	}
	if u.Scheme != "unix" || u.Path == "" {
		return nil, fmt.Errorf("bad endpoint address '%s': must be a UDS path",
			cfg.Endpoint)
	}

	// support for additional FSes requires not only having access to the
	// corresponding `mkfs` tools (which typically means they need to be
	// packaged into the plugin container), but often also adding support
	// for their cmd-line switch quirks to the code, so go easy...
	if cfg.DefaultFS != "ext4" {
		return nil, fmt.Errorf("unsupported default FS: '%s'", cfg.DefaultFS)
	}

	if cfg.Transport != "tcp" && cfg.Transport != "rdma" {
		return nil, fmt.Errorf("unsupported transport type: '%s'", cfg.Transport)
	}

	level, err := logrus.ParseLevel(cfg.LogLevel)
	if err != nil || level < logrus.ErrorLevel || level > logrus.DebugLevel {
		return nil, fmt.Errorf("unsupported log level: '%s'", cfg.LogLevel)
	}
	logger := logrus.New()
	logger.SetLevel(level)
	var logFmt logrus.Formatter
	switch cfg.LogFormat {
	case "json":
		logFmt = &logrus.JSONFormatter{
			DisableTimestamp: !cfg.LogTimestamps,
			PrettyPrint:      cfg.PrettyJson,
			TimestampFormat:  logTimestampFmt,
		}
	case "text":
		logFmt = &logrus.TextFormatter{
			FullTimestamp:   cfg.LogTimestamps,
			TimestampFormat: logTimestampFmt,
		}
	default:
		return nil, fmt.Errorf("unsupported log format: '%s'", cfg.LogFormat)
	}
	logger.SetFormatter(logFmt)
	extraFields := logrus.Fields{"node": cfg.NodeID}
	if cfg.LogRole != "" {
		extraFields["role"] = cfg.LogRole
	}
	log := logger.WithFields(extraFields)

	log.WithFields(logrus.Fields{
		"driver-name":       driverName,
		"config":            fmt.Sprintf("%+v", cfg),
		"version-rel":       version,
		"version-git":       versionGitCommit,
		"version-hash":      versionBuildHash,
		"version-build-id":  versionBuildID,
	}).Info("starting")

	// ok, so this is a bit heavy-handed, but until K8s guys factor it out -
	// it's too good to reimplement from scratch.
	// TODO: actually, should be passed in as part of the config, to allow
	// testing/mocking...
	mounter := &mount.SafeFormatAndMount{
		Interface: mount.New(""),
		Exec:      mount.NewOsExec(),
	}

	lbdialer := func(ctx context.Context, targets endpoint.Slice) (lb.Client, error) {
		return lbgrpc.Dial(ctx, log, targets)
	}

	return &Driver{
		sockPath:      u.Path,
		nodeID:        cfg.NodeID,
		hostNQN:       nodeIDToHostNQN(cfg.NodeID),
		log:           log,
		lbclients:     lb.NewClientPool(lbdialer),
		mounter:       mounter,
		transport:     cfg.Transport,
		squelchPanics: cfg.SquelchPanics,
	}, nil
}

func (d *Driver) Run() error {
	// cleanup leftover socket, if any (e.g. prev instance crash).
	if err := os.Remove(d.sockPath); err != nil && !os.IsNotExist(err) {
		return errors.Wrapf(err, "failed to remove leftover socket file '%s'", d.sockPath)
	}

	listener, err := net.Listen("unix", d.sockPath)
	if err != nil {
		return errors.Wrap(err, "failed to listen on endpoint")
	}

	// TODO: consider making interceptor logging optional (conditional on
	// cmd line switch or env var). though by now, the whole driver relies
	// pretty heavily on requests/replies being logged at gRPC level...
	ctxTagOpts := []grpc_ctxtags.Option{
		//TODO: consider replacing with custom narrow field extractor
		// just for the stuff of interest:
		grpc_ctxtags.WithFieldExtractor(grpc_ctxtags.CodeGenRequestFieldExtractor),
	}
	logrusOpts := []grpc_logrus.Option{
		grpc_logrus.WithLevels(grpcutil.CodeToLogrusLevel),
	}

	interceptors := []grpc.UnaryServerInterceptor{
		grpc_ctxtags.UnaryServerInterceptor(ctxTagOpts...),
		grpc_logrus.UnaryServerInterceptor(d.log, logrusOpts...),
		grpcutil.RespDetailInterceptor,
		grpc_logrus.PayloadUnaryServerInterceptor(d.log,
			func(context.Context, string, interface{}) bool { return true },
		),
	}
	if d.squelchPanics {
		interceptors = append(interceptors, grpc_recovery.UnaryServerInterceptor())
	}

	d.srv = grpc.NewServer(grpc.UnaryInterceptor(
		grpc_middleware.ChainUnaryServer(interceptors...)))

	csi.RegisterIdentityServer(d.srv, d)
	csi.RegisterNodeServer(d.srv, d)
	csi.RegisterControllerServer(d.srv, d)

	d.log.WithField("addr", d.sockPath).Info("server started")
	return d.srv.Serve(listener)
}

// general helpers: ----------------------------------------------------------

func (d *Driver) mungeLBErr(
	log *logrus.Entry, err error, format string, args ...interface{},
) error {
	log.Warn(fmt.Sprintf(format+": %s", append(args, err)...))
	if shouldRetryOn(err) {
		return mkEagain("temporarily "+format, args...)
	}
	// something odd is clearly going on in the LightOS cluster, but
	// given K8s' monomaniacal habit of retrying - not sure this
	// will deter it...
	return mkExternal(format+": %s", append(args, err)...)
}

// LB client pool helpers: ---------------------------------------------------

// GetLBClient conjures up a functional LB mgmt API client by whatever means
// necessary. the errors it returns are gRPC-grade and can be returned directly
// to the remote CSI API clients.
func (d *Driver) GetLBClient(ctx context.Context, mgmtEPs endpoint.Slice) (lb.Client, error) {
	clnt, err := d.lbclients.GetClient(ctx, mgmtEPs)
	if err != nil {
		msg := fmt.Sprintf("failed to connect to LBs at '%s': %s", mgmtEPs, err.Error())
		st, ok := status.FromError(err)
		if !ok {
			// that's highly unusual of lb.ClientPool and probably
			// indicates a bug somewhere in the plugin...
			return nil, mkInternal(msg)
		}
		switch st.Code() {
		case codes.Canceled,
			codes.DeadlineExceeded:
			return nil, err
		}
		// if we failed to connect to a LB for an external, presumably
		// net-related reason, just try to cause the CO to retry the
		// whole thing at a later time:
		return nil, mkEagain(msg)
	}

	err = clnt.RemoteOk(ctx)
	if err != nil {
		d.lbclients.PutClient(clnt)
		return nil, err // TODO: filter as above?
	}

	return clnt, nil
}

// PutLBClient returns a client that necessarily must have been previously
// obtained using GetLBClient, with the understanding that the Driver will
// dispose of it as necessary at its discretion.
func (d *Driver) PutLBClient(clnt lb.Client) {
	d.lbclients.PutClient(clnt)
}
