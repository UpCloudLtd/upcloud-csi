package driver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"

	"github.com/UpCloudLtd/upcloud-go-api/v5/upcloud"
	upcloudclient "github.com/UpCloudLtd/upcloud-go-api/v5/upcloud/client"
	"github.com/UpCloudLtd/upcloud-go-api/v5/upcloud/request"
	upcloudservice "github.com/UpCloudLtd/upcloud-go-api/v5/upcloud/service"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

const (
	// DefaultDriverName defines the driverName that is used in Kubernetes and the CSI
	// system for the canonical, official driverName of this plugin.
	DefaultDriverName = "storage.csi.upcloud.com"
	// DefaultAddress is the default address that the csi plugin will serve its
	// http handler on.
	DefaultAddress = "127.0.0.1:13071"
)

const (
	// storageSizeThreshold is a size value for checking if user can provision additional volumes.
	storageSizeThreshold = 10
	// clientTimeout helps to tune for timeout on requests to UpCloud API. Measurement: seconds.
	clientTimeout = 120
	// healthTimeout specifies a time limit for health HTTP server in seconds.
	healthTimeout = 15
	// InitTimeout specifies a time limit for driver initialization in seconds.
	initTimeout = 30
	// CheckStorageQuotaTimeout specifies a time limit for checking storage quota in seconds.
	checkStorageQuotaTimeout = 30
	// udevDiskTimeout specifies a time limit for waiting disk appear under /dev/disk/by-id.
	udevDiskTimeout = 60
	// udevSettleTimeout specifies a time limit for waiting udev event queue to become empty.
	udevSettleTimeout = 20
)

// Driver implements the following CSI interfaces:
//
//	csi.IdentityServer
//	csi.ControllerServer
//	csi.NodeServer
type Driver struct {
	options *driverOptions

	srv     *grpc.Server
	httpSrv http.Server

	fs  Filesystem
	log *logrus.Entry

	upcloudclient *upcloudservice.Service
	upclouddriver upcloudService

	healthChecker *HealthChecker

	// ready defines whether the driver is ready to function. This value will
	// be used by the `Identity` service via the `Probe()` method.
	readyMu sync.Mutex // protects ready
	ready   bool
}

type driverOptions struct {
	username string
	password string

	driverName   string
	endpoint     string
	address      string
	nodeHost     string
	nodeID       string
	zone         string
	isController bool
}

// NewDriver returns a CSI plugin that contains the necessary gRPC
// interfaces to interact with Kubernetes over unix domain sockets for
// managing Upcloud Block Storage.
func NewDriver(logger *logrus.Logger, options ...func(*driverOptions)) (*Driver, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*initTimeout)
	defer cancel()

	driverOpts := &driverOptions{}

	for _, option := range options {
		option(driverOpts)
	}

	if driverOpts.driverName == "" {
		driverOpts.driverName = DefaultDriverName
	}

	// Authenticate by passing your account login credentials to the client
	c := upcloudclient.New(driverOpts.username, driverOpts.password, upcloudclient.WithTimeout((time.Second * clientTimeout)))

	// Create the service object
	svc := upcloudservice.New(c)
	acc, err := svc.GetAccount(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to login in API: %w", err)
	}

	serverDetails, err := determineServer(ctx, svc, driverOpts.nodeHost)
	if err != nil {
		return nil, fmt.Errorf("unable to list servers: %w", err)
	}

	driverOpts.zone = serverDetails.Server.Zone
	driverOpts.nodeID = serverDetails.Server.UUID

	healthChecker := NewHealthChecker(&upcloudHealthChecker{account: svc.GetAccount})
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "localhost"
	}
	log := logger.WithFields(logrus.Fields{
		"region":   driverOpts.zone,
		"node_id":  driverOpts.nodeHost,
		"hostname": hostname,
	})
	log.WithField("username", acc.UserName).Info("init driver")
	return &Driver{
		options: driverOpts,
		fs:      newNodeFilesystem(log),
		log:     log,

		healthChecker: healthChecker,
		upcloudclient: svc,
		upclouddriver: &upcloudClient{svc: svc},
	}, nil
}

func UseFilesystem(d *Driver, fs Filesystem) (*Driver, error) {
	if d.ready {
		return d, errors.New("Driver is already initialized")
	}
	d.fs = fs
	return d, nil
}

func determineServer(ctx context.Context, svc *upcloudservice.Service, nodeHost string) (*upcloud.ServerDetails, error) {
	servers, err := svc.GetServers(ctx)
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch servers list")
	}
	for _, s := range servers.Servers {
		if nodeHost == s.Hostname {
			r := request.GetServerDetailsRequest{UUID: s.UUID}
			serverdetails, err := svc.GetServerDetails(ctx, &r)
			if err != nil {
				return nil, err
			}
			return serverdetails, nil
		}
	}
	return nil, fmt.Errorf("node %s not found", nodeHost)
}

// Run starts the CSI plugin by communication over the given endpoint.
func (d *Driver) Run() error { //nolint: funlen // TODO: refactor
	u, err := url.Parse(d.options.endpoint)
	if err != nil {
		return fmt.Errorf("unable to parse address: %w", err)
	}

	grpcAddr := path.Join(u.Host, filepath.FromSlash(u.Path))
	if u.Host == "" {
		grpcAddr = filepath.FromSlash(u.Path)
	}

	// CSI plugins talk only over UNIX sockets currently
	if u.Scheme != "unix" {
		return fmt.Errorf("currently only unix domain sockets are supported, have: %s", u.Scheme)
	}
	// remove the socket if it's already there. This can happen if we
	// deploy a new version and the socket was created from the old running
	// plugin.
	d.log.WithField("socket", grpcAddr).Info("removing socket")

	if err := os.Remove(grpcAddr); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove unix domain socket file %s, error: %w", grpcAddr, err)
	}

	grpcListener, err := net.Listen(u.Scheme, grpcAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	if d.options.isController {
		quotaDetails, err := d.checkStorageQuota()
		if err != nil && quotaDetails != nil {
			d.log.WithError(err).WithField("details", quotaDetails).Warn(err)
		} else if err != nil {
			d.log.WithError(err).Error("quota request failed")
		}
	}

	d.srv = grpc.NewServer(grpc.UnaryInterceptor(serverLogMiddleware(d.log)))
	csi.RegisterIdentityServer(d.srv, d)
	csi.RegisterControllerServer(d.srv, d)
	csi.RegisterNodeServer(d.srv, d)

	httpListener, err := net.Listen("tcp", d.options.address)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		err := d.healthChecker.Check(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	d.httpSrv = http.Server{
		Handler:           mux,
		ReadHeaderTimeout: healthTimeout * time.Second,
	}

	d.ready = true // we're now ready to go!
	d.log.WithFields(logrus.Fields{
		"grpc_addr": grpcAddr,
		"http_addr": d.options.address,
	}).Info("starting server")

	var eg errgroup.Group
	eg.Go(func() error {
		return d.srv.Serve(grpcListener)
	})
	eg.Go(func() error {
		return d.httpSrv.Serve(httpListener)
	})

	return eg.Wait()
}

func (d *Driver) checkStorageQuota() (map[string]int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*checkStorageQuotaTimeout)
	defer cancel()
	account, err := d.upcloudclient.GetAccount(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve account details: %w", err)
	}
	ssdLimit := account.ResourceLimits.StorageSSD

	storages, err := d.upcloudclient.GetStorages(ctx, &request.GetStoragesRequest{})
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve server's storage details: %w", err)
	}

	total := 0
	for _, s := range storages.Storages {
		total += s.Size
	}
	sizeAvailable := ssdLimit - total
	if sizeAvailable <= storageSizeThreshold {
		storageDetails := map[string]int{
			"storage_size_quota":           ssdLimit,
			"storage_available_ssd_size":   sizeAvailable,
			"storage_provisioned_ssd_size": total,
		}
		return storageDetails, fmt.Errorf("available storage size may be insufficient for correct work of CSI controller")
	}

	return nil, err
}

// Stop stops the plugin.
func (d *Driver) Stop() {
	d.readyMu.Lock()
	d.ready = false
	d.readyMu.Unlock()

	d.log.Info("server stopped")
	d.srv.Stop()
}

func WithEndpoint(endpoint string) func(*driverOptions) {
	return func(o *driverOptions) {
		o.endpoint = endpoint
	}
}

func WithDriverName(driverName string) func(options *driverOptions) {
	return func(o *driverOptions) {
		o.driverName = driverName
	}
}

func WithAddress(address string) func(options *driverOptions) {
	return func(o *driverOptions) {
		o.address = address
	}
}

func WithUsername(username string) func(*driverOptions) {
	return func(o *driverOptions) {
		o.username = username
	}
}

func WithPassword(password string) func(*driverOptions) {
	return func(o *driverOptions) {
		o.password = password
	}
}

func WithControllerOn(state bool) func(*driverOptions) {
	return func(o *driverOptions) {
		o.isController = state
	}
}

func WithNodeHost(nodeHost string) func(*driverOptions) {
	return func(o *driverOptions) {
		o.nodeHost = nodeHost
	}
}

// When building any packages that import version, pass the build/install cmd
// ldflags like so:
//   go build -ldflags "-X github.com/UpCloudLtd/upcloud-csi/driver.version=0.0.1"

// TODO look at cleaner way to set these :(.
var (
	gitTreeState = "not a git tree" //nolint: gochecknoglobals // set by build
	commit       string             //nolint: gochecknoglobals // set by build
	version      string             //nolint: gochecknoglobals // set by build
)

func GetVersion() string {
	return version
}

// GetCommit returns the current commit hash value, as inserted at build time.
func GetCommit() string {
	return commit
}

// GetTreeState returns the current state of git tree, either "clean" or
// "dirty".
func GetTreeState() string {
	return gitTreeState
}
