package driver

import (
	"github.com/UpCloudLtd/upcloud-csi/internal/service"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	ErrInvalidDriver       = status.Error(codes.InvalidArgument, "cannot figure out driver to use, neither \"tier\" nor \"fileStorage\" parameters given")
	ErrUnimplementedDriver = status.Error(codes.Unimplemented, "driver for given parameters not implemented")
)

type DriverID string

const (
	BlockDriver DriverID = "Block"
)

type Driver interface {
	Controller(svc *service.UpCloudService) (csi.ControllerServer, error)
	Node() (csi.NodeServer, error)
}
