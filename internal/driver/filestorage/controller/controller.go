package controller

import (
	"context"

	"github.com/UpCloudLtd/upcloud-csi/internal/service"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"
)

type controller struct {
	svc    *service.UpCloudService
	logger *logrus.Entry
}

func NewController(svc *service.UpCloudService, logger *logrus.Entry) (csi.ControllerServer, error) {
	return &controller{
		svc:    svc,
		logger: logger,
	}, nil
}

func (c *controller) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (c *controller) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (c *controller) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (c *controller) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (c *controller) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (c *controller) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (c *controller) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (c *controller) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (c *controller) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (c *controller) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (c *controller) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (c *controller) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (c *controller) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	panic("not implemented") // TODO: Implement
}
