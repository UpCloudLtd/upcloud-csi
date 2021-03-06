package driver

import (
	"context"
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/UpCloudLtd/upcloud-go-api/v4/upcloud"
	upcloudclient "github.com/UpCloudLtd/upcloud-go-api/v4/upcloud/client"
	"github.com/UpCloudLtd/upcloud-go-api/v4/upcloud/request"
	upcloudservice "github.com/UpCloudLtd/upcloud-go-api/v4/upcloud/service"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

const (
	// DefaultDriverName defines the driverName that is used in Kubernetes and the CSI
	// system for the canonical, official driverName of this plugin
	DefaultDriverName = "storage.csi.upcloud.com"
	// DefaultAddress is the default address that the csi plugin will serve its
	// http handler on.
	DefaultAddress = "127.0.0.1:13071"
)

const (
	// StorageSizeThreshold is a size value for checking if user can provision
	// additional volumes
	StorageSizeThreshold = 10
	// ClientTimeout helps to tune for timeout on requests to UpCloud API. Measurement: seconds
	ClientTimeout = 30
)

// Driver implements the following CSI interfaces:
//
//   csi.IdentityServer
//   csi.ControllerServer
//   csi.NodeServer
//
type Driver struct {
	options *driverOptions

	// TODO: remove this struct composition after finishing all controller capabilities
	csi.ControllerServer

	srv     *grpc.Server
	httpSrv http.Server

	mounter Mounter
	log     *logrus.Entry

	upcloudclient *upcloudservice.Service
	upclouddriver upcloudService

	healthChecker *HealthChecker

	storage upcloud.Storage
	// ready defines whether the driver is ready to function. This value will
	// be used by the `Identity` service via the `Probe()` method.
	readyMu sync.Mutex // protects ready
	ready   bool
}

type driverOptions struct {
	username string
	password string

	driverName string
	// volumeName is used to pass the volume from
	// `ControllerPublishVolume` to `NodeStageVolume or `NodePublishVolume`
	volumeName string

	endpoint     string
	address      string
	nodeHost     string
	nodeId       string
	zone         string
	upcloudTag   string
	isController bool
}

// NewDriver returns a CSI plugin that contains the necessary gRPC
// interfaces to interact with Kubernetes over unix domain sockets for
// managing Upcloud Block Storage
func NewDriver(options ...func(*driverOptions)) (*Driver, error) {
	driverOpts := &driverOptions{}

	for _, option := range options {
		option(driverOpts)
	}

	if driverOpts.driverName == "" {
		driverOpts.driverName = DefaultDriverName
	}

	// Authenticate by passing your account login credentials to the client
	c := upcloudclient.New(driverOpts.username, driverOpts.password)

	// It is generally a good idea to override the default timeout of the underlying HTTP client since some requests block for longer periods of time
	c.SetTimeout(time.Second * ClientTimeout)

	// Create the service object
	svc := upcloudservice.New(c)
	acc, err := svc.GetAccount()
	if err != nil {
		fmt.Printf("Failed to login in API: %s\n", err)
		os.Exit(1)
	}
	fmt.Printf("%+v", acc)

	serverDetails, err := determineServer(svc, driverOpts.nodeHost)
	if err != nil {
		fmt.Printf("Unable to list servers: %s\n", err)
		os.Exit(1)
	}

	driverOpts.zone = serverDetails.Server.Zone
	driverOpts.nodeId = serverDetails.Server.UUID

	healthChecker := NewHealthChecker(&upcloudHealthChecker{account: svc.GetAccount})
	log := logrus.New().WithFields(logrus.Fields{
		"region":  driverOpts.zone,
		"node_id": driverOpts.nodeHost,
	})

	return &Driver{
		options: driverOpts,
		mounter: newMounter(log),
		log:     log,

		healthChecker: healthChecker,
		upcloudclient: svc,
		upclouddriver: &upcloudClient{svc: svc},
	}, nil
}

func determineServer(svc *upcloudservice.Service, nodeHost string) (*upcloud.ServerDetails, error) {
	servers, err := svc.GetServers()
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch servers list")
	}
	for _, s := range servers.Servers {
		if nodeHost == s.Hostname {
			r := request.GetServerDetailsRequest{UUID: s.UUID}
			serverdetails, err := svc.GetServerDetails(&r)
			if err != nil {
				return nil, err
			}
			return serverdetails, nil
		}
	}
	return nil, fmt.Errorf("node %s not found", nodeHost)
}

// Run starts the CSI plugin by communication over the given endpoint
func (d *Driver) Run() error {
	u, err := url.Parse(d.options.endpoint)
	if err != nil {
		return fmt.Errorf("unable to parse address: %q", err)
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
		return fmt.Errorf("failed to remove unix domain socket file %s, error: %s", grpcAddr, err)
	}

	grpcListener, err := net.Listen(u.Scheme, grpcAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}

	// log response errors for better observability
	errHandler := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)
		if err != nil {
			d.log.WithField("method", info.FullMethod).Errorf("method failed: %s", err)
		}
		return resp, err
	}

	if d.options.isController {
		quotaDetails, err := d.checkStorageQuota()
		if err != nil && quotaDetails != nil {
			d.log.WithFields(logrus.Fields{"details": quotaDetails}).Warn(err)
		} else if err != nil {
			d.log.Errorf("Quota request error: %s", err)
		}
	}

	d.srv = grpc.NewServer(grpc.UnaryInterceptor(errHandler))
	csi.RegisterIdentityServer(d.srv, d)
	csi.RegisterControllerServer(d.srv, d)
	csi.RegisterNodeServer(d.srv, d)

	httpListener, err := net.Listen("tcp", d.options.address)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
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
		Handler: mux,
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
	account, err := d.upcloudclient.GetAccount()
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve account details: %s", err)
	}
	ssdLimit := account.ResourceLimits.StorageSSD

	storages, err := d.upcloudclient.GetStorages(&request.GetStoragesRequest{})
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve server's storage details: %s", err)
	}

	total := 0
	for _, s := range storages.Storages {
		total = total + s.Size
	}
	sizeAvailable := ssdLimit - total
	if sizeAvailable <= StorageSizeThreshold {
		storageDetails := map[string]int{
			"storage_size_quota":           ssdLimit,
			"storage_available_ssd_size":   sizeAvailable,
			"storage_provisioned_ssd_size": total,
		}
		return storageDetails, fmt.Errorf("available storage size may be insufficient for correct work of CSI controller")

	}

	return nil, nil
}

// Stop stops the plugin
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

func WithVolumeName(volumeName string) func(options *driverOptions) {
	return func(o *driverOptions) {
		o.volumeName = volumeName
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

// TODO look at cleaner way to set these :(
var (
	gitTreeState = "not a git tree"
	commit       string
	version      string
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
