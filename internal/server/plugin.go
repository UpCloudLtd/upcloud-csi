package server

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"

	"github.com/UpCloudLtd/upcloud-csi/internal/logger"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

type PluginServer struct {
	srv    *grpc.Server
	log    *logrus.Entry
	listen *url.URL
}

func NewControllerPluginServer(addr string, controllerServer csi.ControllerServer, identity csi.IdentityServer, l *logrus.Entry) (*PluginServer, error) {
	return NewPluginServer(addr, controllerServer, nil, identity, l)
}

func NewNodePluginServer(addr string, nodeServer csi.NodeServer, identity csi.IdentityServer, l *logrus.Entry) (*PluginServer, error) {
	return NewPluginServer(addr, nil, nodeServer, identity, l)
}

func NewPluginServer(addr string, controllerServer csi.ControllerServer, nodeServer csi.NodeServer, identity csi.IdentityServer, l *logrus.Entry) (*PluginServer, error) {
	if identity == nil {
		return nil, errors.New("identity service is not defined")
	}
	listen, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}

	// CSI plugins talk only over UNIX sockets currently
	if listen.Scheme != "unix" {
		log.Fatalf("currently only unix domain sockets are supported, have: %s", listen.Scheme)
	}

	srv := grpc.NewServer(grpc.UnaryInterceptor(logger.NewMiddleware(l)))

	// both controller and node requires identity server
	csi.RegisterIdentityServer(srv, identity)

	if controllerServer != nil {
		csi.RegisterControllerServer(srv, controllerServer)
	}
	if nodeServer != nil {
		csi.RegisterNodeServer(srv, nodeServer)
	}
	return &PluginServer{
		listen: listen,
		srv:    srv,
		log:    l,
	}, nil
}

func (s *PluginServer) cleanUpSocket(socketPath string) error {
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		return nil
	}
	s.log.WithField("socket", socketPath).Info("removing socket")

	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove unix domain socket file %s, error: %w", socketPath, err)
	}
	return nil
}

func (s *PluginServer) Run() error {
	s.log.WithField("listen", s.listen.String()).Info("starting GRPC server")

	// remove the socket if it's already there. This can happen if we
	// deploy a new version and the socket was created from the old running
	// plugin.
	if err := s.cleanUpSocket(s.listen.Path); err != nil {
		return err
	}
	grpcListener, err := net.Listen(s.listen.Scheme, s.listen.Path)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	return s.srv.Serve(grpcListener)
}

// Stop stops the plugin.
func (s *PluginServer) Stop(sig os.Signal) {
	s.log.WithField("signal", sig).Info("stopping GRPC server gracefully")
	if s.srv != nil {
		s.srv.GracefulStop()
	}
	if s.listen != nil {
		if err := s.cleanUpSocket(s.listen.Path); err != nil {
			s.log.Error(err)
		}
	}
}
