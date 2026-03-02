package plugin

import (
	"context"
	"os"
	"time"

	"github.com/UpCloudLtd/upcloud-csi/internal/driver"
	"github.com/UpCloudLtd/upcloud-csi/internal/driver/block"
	"github.com/UpCloudLtd/upcloud-csi/internal/filesystem"
	"github.com/UpCloudLtd/upcloud-csi/internal/identity"
	"github.com/UpCloudLtd/upcloud-csi/internal/logger"
	"github.com/UpCloudLtd/upcloud-csi/internal/plugin/config"
	"github.com/UpCloudLtd/upcloud-csi/internal/server"
	"github.com/UpCloudLtd/upcloud-csi/internal/service"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"
)

func Run(c config.Config) error {
	l := logger.New(c.LogLevel).WithField(logger.HostKey, hostname())
	healthServer, err := server.NewHealthServer(c.HealtServerAddress, l)
	if err != nil {
		return err
	}

	pluginServer, err := newPluginServer(c, l)
	if err != nil {
		return err
	}
	return server.Run(pluginServer, healthServer)
}

func newPluginServer(c config.Config, l *logrus.Entry) (*server.PluginServer, error) {
	var err error
	if c.Filesystem == nil {
		c.Filesystem, err = filesystem.NewLinuxFilesystem(c.FilesystemTypes, l)
		if err != nil {
			return nil, err
		}
	}

	driver, err := initDriver(&c, l)
	if err != nil {
		return nil, err
	}

	var csiController csi.ControllerServer
	var csiNode csi.NodeServer

	var svc *service.UpCloudService
	if c.Username != "" {
		svc, err = service.NewUpCloudServiceFromCredentials(c.Username, c.Password)
		if err != nil {
			return nil, err
		}

		autoConfigureZone(svc, &c)
	}

	if c.Mode == config.DriverModeController || c.Mode == config.DriverModeMonolith {
		csiController, err = driver.Controller(svc)
		if err != nil {
			return nil, err
		}
	}

	if c.Mode == config.DriverModeNode || c.Mode == config.DriverModeMonolith {
		csiNode, err = driver.Node()
		if err != nil {
			return nil, err
		}
	}

	identity := identity.NewIdentity(c.DriverName, l)
	pluginServer, err := server.NewPluginServer(c.PluginServerAddress, csiController, csiNode, identity, l)
	if err != nil {
		return nil, err
	}
	return pluginServer, nil
}

func autoConfigureZone(svc *service.UpCloudService, c *config.Config) {
	if c.Zone == "" && c.NodeHost != "" {
		// if zone is not provided, try to use nodeHost to auto-configure zone
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if srv, err := svc.GetServerByHostname(ctx, c.NodeHost); err == nil {
			c.Zone = srv.Zone
		}
	}
}

func initDriver(c *config.Config, l *logrus.Entry) (driver.Driver, error) {
	switch c.Driver {
	case string(driver.BlockDriver):
		return block.New(c, l.WithField(logger.DriverKey, "block"))
	default:
		return nil, errors.New("invalid driver")
	}
}

func hostname() string {
	if n, err := os.Hostname(); err == nil {
		return n
	}
	return ""
}
