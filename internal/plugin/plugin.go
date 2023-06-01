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

	switch c.Mode {
	case config.DriverModeController:
		return runController(c, healthServer, l)
	case config.DriverModeNode:
		return runNode(c, healthServer, l)
	case config.DriverModeMonolith:
		return runMonolith(c, healthServer, l)
	}
	return fmt.Errorf("unknow driver mode '%s'", c.Mode)
}

func runNode(c config.Config, healthServer *server.HealthServer, l *logrus.Entry) error {
	l = l.WithField(logger.NodeIDKey, c.NodeHost)
	if c.Zone != "" {
		l = l.WithField(logger.ZoneKey, c.Zone)
	}

	csiNode, err := node.NewNode(c.NodeHost, c.Zone, int64(config.MaxVolumesPerNode), filesystem.NewLinuxFilesystem(l), l)
	if err != nil {
		return err
	}
	identity := identity.NewIdentity(c.DriverName, l)
	pluginServer, err := server.NewNodePluginServer(c.PluginServerAddress, csiNode, identity, l)
	if err != nil {
		return err
	}
	return server.Run(pluginServer, healthServer)
}

func runController(c config.Config, healthServer *server.HealthServer, l *logrus.Entry) error {
	svc, err := service.NewUpCloudServiceFromCredentials(c.Username, c.Password)
	if err != nil {
		return err
	}

	autoConfigureZone(svc, &c)
	l = l.WithField(logger.ZoneKey, c.Zone)
	csiController, err := controller.NewController(svc, c.Zone, config.MaxVolumesPerNode, l, c.Labels...)
	if err != nil {
		return err
	}
	identity := identity.NewIdentity(c.DriverName, l)
	pluginServer, err := server.NewControllerPluginServer(c.PluginServerAddress, csiController, identity, l)
	if err != nil {
		return err
	}
	return server.Run(pluginServer, healthServer)
}

func runMonolith(c config.Config, healthServer *server.HealthServer, l *logrus.Entry) error {
	svc, err := service.NewUpCloudServiceFromCredentials(c.Username, c.Password)
	if err != nil {
		return err
	}
	autoConfigureZone(svc, &c)
	l = l.WithField(logger.NodeIDKey, c.NodeHost).WithField(logger.ZoneKey, c.Zone)
	csiController, err := controller.NewController(svc, c.Zone, config.MaxVolumesPerNode, l, c.Labels...)
	if err != nil {
		return err
	}
	csiNode, err := node.NewNode(c.NodeHost, c.Zone, int64(config.MaxVolumesPerNode), filesystem.NewLinuxFilesystem(l), l)
	if err != nil {
		return err
	}
	identity := identity.NewIdentity(c.DriverName, l)
	pluginServer, err := server.NewPluginServer(c.PluginServerAddress, csiController, csiNode, identity, l)
	if err != nil {
		return err
	}
	return server.Run(pluginServer, healthServer)
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
