package driver

import (
	"context"
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/gemalto/flume"
	"k8s.io/utils/exec"
	"k8s.io/utils/mount"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/UpCloudLtd/upcloud-go-api/upcloud"
	upcloudclient "github.com/UpCloudLtd/upcloud-go-api/upcloud/client"
	"github.com/UpCloudLtd/upcloud-go-api/upcloud/request"
	upcloudservice "github.com/UpCloudLtd/upcloud-go-api/upcloud/service"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

const (
	// DefaultDriverName defines the name that is used in Kubernetes and the CSI
	// system for the canonical, official name of this plugin
	DefaultDriverName = "csi.upcloud.com"
	// DefaultAddress is the default address that the csi plugin will serve its
	// http handler on.
	DefaultAddress = "127.0.0.1:13071"
)

// Driver implements the following CSI interfaces:
//
//   csi.IdentityServer
//   csi.ControllerServer
//   csi.NodeServer
//
type Driver struct {
	name string
	// volumeName is used to pass the volume name from
	// `ControllerPublishVolume` to `NodeStageVolume or `NodePublishVolume`
	volumeName string

	endpoint     string
	address      string
	nodeId       string
	zone         string
	upcloudTag   string
	isController bool

	identitySvc   *IdentityService
	nodeSvc       *NodeService
	controllerSvc *ControllerService

	srv     *grpc.Server
	httpSrv http.Server
	mounter *mount.SafeFormatAndMount
	log     flume.Logger

	upcloudclient *upcloudservice.Service
	upclouddriver upcloudClient

	healthChecker *HealthChecker

	// ready defines whether the driver is ready to function. This value will
	// be used by the `Identity` service via the `Probe()` method.
	readyMu sync.Mutex // protects ready
	ready   bool
}

// NewDriver returns a CSI plugin that contains the necessary gRPC
// interfaces to interact with Kubernetes over unix domain sockets for
// managing Upcloud Block Storage
func NewDriver(ep, username, password, url, nodeId, driverName, address string) (*Driver, error) {
	if driverName == "" {
		driverName = DefaultDriverName
	}

	// Authenticate by passing your account login credentials to the client
	c := upcloudclient.New(username, password)

	// It is generally a good idea to override the default timeout of the underlying HTTP client since some requests block for longer periods of time
	c.SetTimeout(time.Second * 30)

	// Create the service object
	svc := upcloudservice.New(c)
	acc, err := svc.GetAccount()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("%+v", acc)

	serverdetails, err := determineServer(svc, nodeId)
	if err != nil {
		panic(err)
	}

	region := serverdetails.Server.Zone
	nodeServerUUID := serverdetails.Server.UUID

	healthChecker := NewHealthChecker(&upcloudHealthChecker{account: svc.GetAccount})
	log := flume.New("upcloud-csi").With("region", region).With("node_id", nodeId)

	return &Driver{
		name:       driverName,
		volumeName: driverName + "/volume-name",

		endpoint: ep,
		address:  address,
		nodeId:   nodeServerUUID,
		zone:     region,
		// for now we're assuming only the controller has a non-empty token. In
		// the future we should pass an explicit flag to the driver.
		isController: password != "",
		mounter: &mount.SafeFormatAndMount{
			Interface: mount.New(""),
			Exec:      exec.New(),
		},
		log: log,

		healthChecker: healthChecker,
		upcloudclient: svc,
		upclouddriver: upcloudClient{svc: svc},
	}, nil
}

func determineServer(svc *upcloudservice.Service, nodeId string) (*upcloud.ServerDetails, error) {
	servers, _ := svc.GetServers()
	for _, s := range servers.Servers {
		if nodeId == s.Hostname {
			r := request.GetServerDetailsRequest{UUID: s.UUID}
			serverdetails, _ := svc.GetServerDetails(&r)
			return serverdetails, nil
		}
	}
	return nil, fmt.Errorf("node %s not found", nodeId)
}

// Run starts the CSI plugin by communication over the given endpoint
func (d *Driver) Run() error {
	u, err := url.Parse(d.endpoint)
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
	d.log.With("socket", grpcAddr).Info("removing socket")

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
			d.log.With("method", info.FullMethod).Error("method failed", err)
		}
		return resp, err
	}

	// warn the user, it'll not propagate to the user but at least we see if
	// something is wrong in the logs. Only check if the driver is running with
	// a token (i.e: controller)
	// if d.isController {
	// 	if err := d.checkLimit(context.Background()); err != nil {
	// 		d.log.WithError(err).Warn("CSI plugin will not function correctly, please resolve volume limit")
	// 	}
	// }

	d.srv = grpc.NewServer(grpc.UnaryInterceptor(errHandler))
	csi.RegisterIdentityServer(d.srv, d.identitySvc)
	csi.RegisterControllerServer(d.srv, d.controllerSvc)
	csi.RegisterNodeServer(d.srv, d.nodeSvc)

	httpListener, err := net.Listen("tcp", d.address)
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
	d.log.With("grpc_addr", grpcAddr, "http_addr", d.address).Info("starting server")

	var eg errgroup.Group
	eg.Go(func() error {
		return d.srv.Serve(grpcListener)
	})
	eg.Go(func() error {
		return d.httpSrv.Serve(httpListener)
	})

	return eg.Wait()
}

// Stop stops the plugin
func (d *Driver) Stop() {
	d.readyMu.Lock()
	d.ready = false
	d.readyMu.Unlock()

	d.log.Info("server stopped")
	d.srv.Stop()
}
