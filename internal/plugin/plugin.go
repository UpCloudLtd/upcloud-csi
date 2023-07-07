package plugin

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/UpCloudLtd/upcloud-csi/internal/controller"
	"github.com/UpCloudLtd/upcloud-csi/internal/filesystem"
	"github.com/UpCloudLtd/upcloud-csi/internal/identity"
	"github.com/UpCloudLtd/upcloud-csi/internal/logger"
	"github.com/UpCloudLtd/upcloud-csi/internal/node"
	"github.com/UpCloudLtd/upcloud-csi/internal/plugin/config"
	"github.com/UpCloudLtd/upcloud-csi/internal/server"
	"github.com/UpCloudLtd/upcloud-csi/internal/service"
	"github.com/sirupsen/logrus"
)

func Run(c config.Config) error {
	l := logger.New(c.LogLevel).WithField(logger.HostKey, hostname())
	healthServer, err := server.NewHealthServer(c.HealtServerAddress, l)
	if err != nil {
		return err
	}

	srv, err := newPluginServer(c, l)
	if err != nil {
		return err
	}
	return server.Run(srv, healthServer)
}

func newPluginServer(c config.Config, l *logrus.Entry) (*server.PluginServer, error) {
	var srv *server.PluginServer
	var err error
	if c.Filesystem == nil {
		c.Filesystem = filesystem.NewLinuxFilesystem(l)
	}
	switch c.Mode {
	case config.DriverModeController:
		if srv, err = newControllerPluginServer(c, l); err != nil {
			return srv, err
		}
	case config.DriverModeNode:
		if srv, err = newNodePluginServer(c, l); err != nil {
			return srv, err
		}
	case config.DriverModeMonolith:
		if srv, err = newMonolithPluginServer(c, l); err != nil {
			return srv, err
		}
	default:
		return srv, fmt.Errorf("unknow driver mode '%s'", c.Mode)
	}
	return srv, nil
}

func newNodePluginServer(c config.Config, l *logrus.Entry) (*server.PluginServer, error) {
	l = l.WithField(logger.NodeIDKey, c.NodeHost)
	if c.Zone != "" {
		l = l.WithField(logger.ZoneKey, c.Zone)
	}

	csiNode, err := node.NewNode(c.NodeHost, c.Zone, int64(config.MaxVolumesPerNode), c.Filesystem, l)
	if err != nil {
		return nil, err
	}
	identity := identity.NewIdentity(c.DriverName, l)
	pluginServer, err := server.NewNodePluginServer(c.PluginServerAddress, csiNode, identity, l)
	if err != nil {
		return nil, err
	}
	return pluginServer, nil
}

func newControllerPluginServer(c config.Config, l *logrus.Entry) (*server.PluginServer, error) {
	svc, err := service.NewUpCloudServiceFromCredentials(c.Username, c.Password)
	if err != nil {
		return nil, err
	}

	autoConfigureZone(svc, &c)
	l = l.WithField(logger.ZoneKey, c.Zone)
	csiController, err := controller.NewController(svc, c.Zone, config.MaxVolumesPerNode, l, c.Labels...)
	if err != nil {
		return nil, err
	}
	identity := identity.NewIdentity(c.DriverName, l)
	pluginServer, err := server.NewControllerPluginServer(c.PluginServerAddress, csiController, identity, l)
	if err != nil {
		return nil, err
	}
	return pluginServer, nil
}

func newMonolithPluginServer(c config.Config, l *logrus.Entry) (*server.PluginServer, error) {
	svc, err := service.NewUpCloudServiceFromCredentials(c.Username, c.Password)
	if err != nil {
		return nil, err
	}
	autoConfigureZone(svc, &c)
	l = l.WithField(logger.NodeIDKey, c.NodeHost).WithField(logger.ZoneKey, c.Zone)
	csiController, err := controller.NewController(svc, c.Zone, config.MaxVolumesPerNode, l, c.Labels...)
	if err != nil {
		return nil, err
	}
	csiNode, err := node.NewNode(c.NodeHost, c.Zone, int64(config.MaxVolumesPerNode), c.Filesystem, l)
	if err != nil {
		return nil, err
	}
	identity := identity.NewIdentity(c.DriverName, l)
	pluginServer, err := server.NewPluginServer(c.PluginServerAddress, csiController, csiNode, identity, l)
	if err != nil {
		return nil, err
	}
	return pluginServer, nil
}

func autoConfigureZone(svc *service.UpCloudService, c *config.Config) {
	if c.Zone == "" {
		// if zone is not provided, try to use nodeHost to auto-configure zone
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if srv, err := svc.GetServerByHostname(ctx, c.NodeHost); err == nil {
			c.Zone = srv.Zone
		}
	}
}

func hostname() string {
	if n, err := os.Hostname(); err == nil {
		return n
	}
	return ""
}
