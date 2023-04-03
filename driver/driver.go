package driver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/UpCloudLtd/upcloud-go-api/v6/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v6/upcloud/request"
	upsvc "github.com/UpCloudLtd/upcloud-go-api/v6/upcloud/service"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

const (
	// DefaultDriverName defines the name that is used in Kubernetes and the CSI
	// system for the canonical, official name of this plugin.
	DefaultDriverName = "storage.csi.upcloud.com"
	// DefaultAddress is the default address that the csi plugin will serve its
	// http handler on.
	DefaultAddress = "tcp://127.0.0.1:13071"
	// DefaultEndpoint is the default endpoint that the csi plugin will serve its
	// GRPC handlers on.
	DefaultEndpoint = "unix:///var/lib/kubelet/plugins/" + DefaultDriverName + "/csi.sock"
	// storageSizeThreshold is a size value for checking if user can provision additional volumes.
	storageSizeThreshold = 10
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
	options Options
	grpcSrv *grpc.Server
	httpSrv *http.Server
	fs      Filesystem
	log     *logrus.Entry
	svc     service

	// ready defines whether the driver is ready to function. This value will
	// be used by the `Identity` service via the `Probe()` method.
	readyMu sync.Mutex // protects ready
	ready   bool
}

type Options struct {
	DriverName    string
	Endpoint      *url.URL
	Address       *url.URL
	NodeHost      string
	Zone          string
	IsController  bool
	StorageLabels []upcloud.Label
}

// NewDriver returns a CSI plugin that contains the necessary gRPC
// interfaces to interact with Kubernetes over unix domain sockets for
// managing Upcloud Block Storage.
func NewDriver(svc *upsvc.Service, options Options, logger *logrus.Logger, fs Filesystem) (*Driver, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*initTimeout)
	defer cancel()

	if options.DriverName == "" {
		options.DriverName = DefaultDriverName
	}
	if options.Endpoint == nil {
		options.Endpoint, _ = url.Parse(DefaultEndpoint)
	}
	if options.Address == nil {
		options.Address, _ = url.Parse(DefaultAddress)
	}
	account, err := svc.GetAccount(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to login in API: %w", err)
	}
	logger.WithField("username", account.UserName).Info("init driver")

	srv, err := getNodeHost(ctx, svc, options.NodeHost)
	if err != nil {
		logger.WithField("hostname", options.NodeHost).WithError(err).Warn("unable to find server by hostname")
	} else if options.Zone == "" {
		options.Zone = srv.Zone
	}
	if err := validateZone(ctx, svc, options.Zone); err != nil {
		return nil, err
	}

	if options.IsController {
		if quotaDetails, err := checkStorageQuota(svc, account); err != nil {
			logger.WithError(err).WithField("details", quotaDetails).Warn(err)
		}
	}
	if fs == nil {
		fs = newNodeFilesystem(logEntyFromOptions(logger, options))
	}
	return &Driver{
		options: options,
		fs:      fs,
		log:     logEntyFromOptions(logger, options),
		svc:     &upCloudService{svc: svc},
	}, nil
}

func (d *Driver) cleanUpSocket(socketPath string) error {
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		return nil
	}
	d.log.WithField("socket", socketPath).Info("removing socket")

	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove unix domain socket file %s, error: %w", socketPath, err)
	}
	return nil
}

// Run starts the CSI plugin by communication over the given endpoint.
func (d *Driver) Run(checks ...HealthCheck) error {
	// remove the socket if it's already there. This can happen if we
	// deploy a new version and the socket was created from the old running
	// plugin.
	if err := d.cleanUpSocket(d.options.Endpoint.Path); err != nil {
		return err
	}

	d.log.WithFields(logrus.Fields{
		"endpoint":   d.options.Endpoint.String(),
		"health_url": fmt.Sprintf("http://%s/health", d.options.Address.Host),
	}).Info("starting servers")

	d.ready = true
	var eg errgroup.Group
	eg.Go(func() error {
		return d.runGRPC()
	})
	eg.Go(func() error {
		return d.runHTTP(checks...)
	})
	go d.listenSyscalls()
	return eg.Wait()
}

func (d *Driver) runHTTP(checks ...HealthCheck) error {
	httpListener, err := net.Listen(d.options.Address.Scheme, d.options.Address.Host)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	healthChecker := NewHealthChecker(checks...)
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		err := healthChecker.Check(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	d.httpSrv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: healthTimeout * time.Second,
	}
	return d.httpSrv.Serve(httpListener)
}

func (d *Driver) runGRPC() error {
	grpcListener, err := net.Listen(d.options.Endpoint.Scheme, d.options.Endpoint.Path)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	d.grpcSrv = grpc.NewServer(grpc.UnaryInterceptor(serverLogMiddleware(d.log)))
	csi.RegisterIdentityServer(d.grpcSrv, d)
	csi.RegisterControllerServer(d.grpcSrv, d)
	csi.RegisterNodeServer(d.grpcSrv, d)

	return d.grpcSrv.Serve(grpcListener)
}

func (d *Driver) listenSyscalls() {
	// Listen for syscall signals for process to interrupt/quit
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	sig := <-sigCh
	signal.Stop(sigCh)
	d.log.WithField("signal", sig).Info("received OS signal shutting down")
	d.Stop()
	close(sigCh)
}

// Stop stops the plugin.
func (d *Driver) Stop() {
	d.readyMu.Lock()
	d.ready = false
	d.readyMu.Unlock()

	d.log.WithField("health_url", fmt.Sprintf("http://%s/health", d.options.Address.Host)).Info("stopping HTTP server")
	if err := d.httpSrv.Close(); err != nil {
		d.log.Error(err)
	}
	d.log.WithField("endpoint", d.options.Endpoint.String()).Info("stopping GRPC server gracefully")
	d.grpcSrv.GracefulStop()
	if err := d.cleanUpSocket(d.options.Endpoint.Path); err != nil {
		d.log.Error(err)
	}
}

func validateZone(ctx context.Context, svc *upsvc.Service, zone string) error {
	if zone == "" {
		return errors.New("zone is not selected")
	}
	r, err := svc.GetZones(ctx)
	if err != nil {
		return err
	}
	for _, z := range r.Zones {
		if z.ID == zone {
			return nil
		}
	}
	return fmt.Errorf("'%s' is not valid UpCloud zone", zone)
}

func checkStorageQuota(svc *upsvc.Service, account *upcloud.Account) (map[string]int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*checkStorageQuotaTimeout)
	defer cancel()

	storageDetails := make(map[string]int)
	ssdLimit := account.ResourceLimits.StorageSSD

	storages, err := svc.GetStorages(ctx, &request.GetStoragesRequest{})
	if err != nil {
		return storageDetails, fmt.Errorf("unable to retrieve server's storage details: %w", err)
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

	return storageDetails, err
}

func getNodeHost(ctx context.Context, svc *upsvc.Service, nodeHost string) (*upcloud.Server, error) {
	servers, err := svc.GetServers(ctx)
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch servers list")
	}
	for _, s := range servers.Servers {
		if nodeHost == s.Hostname {
			return &s, nil
		}
	}
	return nil, fmt.Errorf("CSI user account doesn't have node with hostname '%s' defined", nodeHost)
}

func logEntyFromOptions(logger *logrus.Logger, options Options) *logrus.Entry {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "localhost"
	}
	return logger.WithFields(logrus.Fields{
		"region":   options.Zone,
		"node_id":  options.NodeHost,
		"hostname": hostname,
	})
}
