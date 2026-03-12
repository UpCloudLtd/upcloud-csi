package filestorage

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"

	"github.com/UpCloudLtd/upcloud-csi/internal/driver"
	"github.com/UpCloudLtd/upcloud-csi/internal/driver/filestorage/controller"
	"github.com/UpCloudLtd/upcloud-csi/internal/driver/filestorage/node"
	"github.com/UpCloudLtd/upcloud-csi/internal/plugin/config"
	"github.com/UpCloudLtd/upcloud-csi/internal/service"
)

type fileStorageDriver struct {
	config *config.Config
	logger *logrus.Entry
}

func New(c *config.Config, l *logrus.Entry) (driver.Driver, error) {
	return fileStorageDriver{
		config: c,
		logger: l,
	}, nil
}

func (d fileStorageDriver) Controller(svc *service.UpCloudService) (csi.ControllerServer, error) {
	return controller.NewController(svc, d.logger)
}

func (d fileStorageDriver) Node() (csi.NodeServer, error) {
	return node.NewNode(d.config.NodeHost, d.config.Zone, d.config.Filesystem, d.logger)
}
