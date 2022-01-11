package driver

import (
	"context"
	"github.com/container-storage-interface/spec/lib/go/csi"
)

type ControllerService struct {
	Driver *Driver
	csi.ControllerServer
}

// CreateVolume provisions storage via UpCloud Storage service
func (controller *ControllerService) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (resp *csi.CreateVolumeResponse, err error) {

	return nil, err
}

func (controller *ControllerService) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (resp *csi.DeleteVolumeResponse, err error) {

	return nil, err
}

func (controller *ControllerService) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (resp *csi.ListVolumesResponse, err error) {

	return nil, err
}

func (controller *ControllerService) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (resp *csi.ControllerGetCapabilitiesResponse, err error) {

	return nil, err
}

func (controller *ControllerService) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (resp *csi.ControllerPublishVolumeResponse, err error) {

	return nil, err
}
