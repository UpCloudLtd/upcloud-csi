package block

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"

	"github.com/UpCloudLtd/upcloud-csi/internal/driver"
	"github.com/UpCloudLtd/upcloud-csi/internal/driver/block/controller"
	"github.com/UpCloudLtd/upcloud-csi/internal/driver/block/node"
	"github.com/UpCloudLtd/upcloud-csi/internal/plugin/config"
	"github.com/UpCloudLtd/upcloud-csi/internal/service"
)

type blockDriver struct {
	config *config.Config
	logger *logrus.Entry
}

func New(c *config.Config, l *logrus.Entry) (driver.Driver, error) {
	return blockDriver{
		config: c,
		logger: l,
	}, nil
}

func (bd blockDriver) Controller(svc *service.UpCloudService) (csi.ControllerServer, error) {
	return controller.NewController(svc, bd.config.Zone, config.MaxVolumesPerNode, bd.logger, bd.config.Labels...)
}

func (bd blockDriver) Node() (csi.NodeServer, error) {
	return node.NewNode(bd.config.NodeHost, bd.config.Zone, int64(config.MaxVolumesPerNode), bd.config.Filesystem, bd.logger)
}
