package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/UpCloudLtd/upcloud-csi/internal/driver/filestorage/common"
	"github.com/UpCloudLtd/upcloud-csi/internal/logger"
	"github.com/UpCloudLtd/upcloud-csi/internal/service"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type controller struct {
	svc    service.FileStorage
	logger *logrus.Entry
}

func NewController(svc service.FileStorage, logger *logrus.Entry) (csi.ControllerServer, error) {
	return &controller{
		svc:    svc,
		logger: logger,
	}, nil
}

func (c *controller) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	if err := c.validateCapabilities(req.VolumeCapabilities); err != nil {
		return nil, err
	}

	fileStorageID, shareName, err := c.parseVolumeID(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	fileStorage, err := c.svc.GetFileStorageByID(ctx, fileStorageID)
	if err != nil {
		if errors.Is(err, service.ErrFileStorageNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	ret := csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeContext: make(map[string]string),
			VolumeCapabilities: []*csi.VolumeCapability{
				{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: "nfs",
						},
					},
				},
			},
		},
	}

	for i := range fileStorage.Shares {
		if fileStorage.Shares[i].Name == shareName {
			return &ret, nil
		}
	}

	return nil, status.Error(codes.NotFound, "did not find given share on given File Storage")
}

func (c *controller) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "no volume name given")
	}

	if err := c.validateCapabilities(req.VolumeCapabilities); err != nil {
		return nil, err
	}

	shareName := req.Name

	fileStorageID, ok := req.Parameters["fileStorage"]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "no fileStorage parameter given")
	}

	logger := c.logger.WithField("fileStorage", fileStorageID)

	fileStorage, err := c.svc.GetFileStorageByID(ctx, fileStorageID)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "cannot fetch File Storage: %v", err.Error())
	}

	for _, requisite := range req.AccessibilityRequirements.GetRequisite() {
		if region, ok := requisite.Segments["region"]; ok && region != fileStorage.Zone {
			return nil, status.Error(codes.FailedPrecondition, "Configured File Storage is not in the zone specified in topology requirements")
		}
	}

	res := csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			CapacityBytes: 0,
			VolumeId:      fmt.Sprintf("%v/%v", fileStorageID, shareName),
			VolumeContext: make(map[string]string),
			AccessibleTopology: []*csi.Topology{
				{
					Segments: map[string]string{
						"region": fileStorage.Zone,
					},
				},
			},
		},
	}

	shareExists := false
	for i := range fileStorage.Shares {
		if fileStorage.Shares[i].Name == shareName {
			shareExists = true
			break
		}
	}

	if !shareExists {
		logger.WithField("shareName", shareName).Debug("Creating Share on FileStorage")
		if err := c.svc.CreateShareOnFileStorage(ctx, fileStorageID, shareName); err != nil {
			return nil, status.Errorf(codes.Internal, "error creating share on File Storage: %s", err.Error())
		}
	}

	return &res, nil
}

func (c *controller) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "no volume ID given")
	}

	fileStorageID, shareName, err := c.parseVolumeID(req.VolumeId)
	if err != nil {
		logger.WithServerContext(ctx, c.logger).WithError(err).Info("invalid volume ID, returning success as per CSI spec")
		return &csi.DeleteVolumeResponse{}, nil
	}

	ret := csi.DeleteVolumeResponse{}

	if err := c.svc.DeleteFileStorageShareByIDAndName(ctx, fileStorageID, shareName); err != nil {
		if errors.Is(err, service.ErrFileStorageShareNotFound) {
			c.logger.Info("was asked to delete a share, but it's already gone")
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &ret, nil
}

func (c *controller) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	if req.NodeId == "" || req.VolumeId == "" || req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "no node ID/volume ID and/or volume capability provided")
	}

	ret := csi.ControllerPublishVolumeResponse{
		PublishContext: make(map[string]string),
	}

	fileStorageID, shareName, err := c.parseVolumeID(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}

	fileStorage, err := c.svc.GetFileStorageByID(ctx, fileStorageID)
	if err != nil {
		if errors.Is(err, service.ErrFileStorageNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	server, err := c.svc.GetServerByHostname(ctx, req.NodeId)
	if err != nil {
		if errors.Is(err, service.ErrServerNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	if server.Zone != fileStorage.Zone {
		return nil, status.Error(codes.InvalidArgument, "Server is not in the same zone as the File Storage")
	}

	network, serverIP, err := c.findServerInterfaceTowardsFileStorage(fileStorage, server)
	if err != nil {
		return nil, err
	}

	// actually only a single network can be attached to a File Storage, hope
	// that limitation is lifted eventually and until then, we don't actually
	// need it ..
	address, err := c.svc.AttachNetworkToFileStorage(ctx, fileStorageID, network)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error attaching network to File Storage: %v", err.Error())
	}
	ret.PublishContext[common.FileStorageAddressPublishContextKey] = address
	ret.PublishContext[common.ShareNamePublishContextKey] = shareName

	c.logger.WithField("address", serverIP).Debug("ensuring ACL for node")
	if err := c.svc.EnsureFileStorageShareACL(ctx, fileStorageID, shareName, req.NodeId, serverIP); err != nil {
		return nil, status.Errorf(codes.Internal, "error ensuring Node ACL for Share on File Storage: %v", err.Error())
	}

	return &ret, nil
}

func (c *controller) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	fileStorageID, shareName, err := c.parseVolumeID(req.VolumeId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err := c.svc.RemoveFileStorageShareACL(ctx, fileStorageID, shareName, req.NodeId); err != nil {
		return nil, status.Errorf(codes.Internal, "error removing Node ACL for Share on File Storage: %v", err.Error())
	}

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (c *controller) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	supportedCapabilities := []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
	}

	caps := make([]*csi.ControllerServiceCapability, 0)
	for _, capability := range supportedCapabilities {
		caps = append(caps, &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: capability,
				},
			},
		})
	}

	logger.WithServerContext(ctx, c.logger).WithField("caps", caps).Info("reporting capabilities")
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: caps,
	}, nil
}

func (c *controller) parseVolumeID(volumeID string) (fileStorage string, share string, err error) {
	parts := strings.SplitN(volumeID, "/", 2)

	if len(parts) != 2 {
		err = errors.New("volumeID has invalid format")
		c.logger.WithError(err).WithField("volumeID", volumeID).Error()
		return
	}

	fileStorage = parts[0]
	share = parts[1]
	return
}

func (c *controller) validateCapabilities(capabilities []*csi.VolumeCapability) error {
	if len(capabilities) == 0 {
		return status.Error(codes.InvalidArgument, "no volume capabilities given")
	}

	for _, capability := range capabilities {
		if capability.GetBlock() != nil {
			return status.Error(codes.InvalidArgument, "cannot create block volumes on File Storage")
		}
	}

	return nil
}

func (c *controller) findServerInterfaceTowardsFileStorage(fileStorage *upcloud.FileStorage, server *upcloud.ServerDetails) (networkUUID string, serverIP string, err error) {
	candidateNetworks := make([]upcloud.ServerInterface, 0, len(server.Networking.Interfaces))
	for _, network := range server.Networking.Interfaces {
		if network.Type == upcloud.NetworkTypePrivate {
			candidateNetworks = append(candidateNetworks, network)
		}
	}

	if len(candidateNetworks) == 0 {
		err = status.Error(codes.InvalidArgument, "Server is not connected to any private Network")
		return
	}

	// if no overlap between networks on the File Storage and networks on the
	// Server, just take the first from the server and hope we can attach it to
	// the File Storage
	chosenNetwork := candidateNetworks[0]

	// .. but let's see if there is any candidate network (= attached to the
	// server) that is already attached to the file storage
findBestNetwork:
	for _, fileStorageNetwork := range fileStorage.Networks {
		for _, candidateNetwork := range candidateNetworks {
			if fileStorageNetwork.UUID == candidateNetwork.Network {
				chosenNetwork = candidateNetwork
				break findBestNetwork
			}
		}
	}

	networkUUID = chosenNetwork.Network

	for _, address := range chosenNetwork.IPAddresses {
		if address.Family == upcloud.IPAddressFamilyIPv6 {
			continue
		}

		serverIP = address.Address
		return
	}

	err = status.Errorf(codes.Internal, "cannot figure out server IP to use")
	return
}

func (c *controller) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	// ALPHA FEATURE
	// This optional RPC MAY be called by the CO to fetch current information about a volume.
	// A Controller Plugin MUST implement this ControllerGetVolume RPC call if it has GET_VOLUME capability.
	// When implemented add csi.ControllerServiceCapability_RPC_GET_VOLUME to supportedCapabilities.
	return nil, status.Errorf(codes.Unimplemented, "method ControllerGetVolume not implemented")
}

func (c *controller) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ListVolumes is not implemented for the File Storage driver")
}

func (c *controller) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, "GetCapacity is not implemented for the File Storage driver")
}

func (c *controller) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "CreateSnapshot is not implemented for the File Storage driver")
}

func (c *controller) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "DeleteSnapshot is not implemented for the File Storage driver")
}

func (c *controller) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ListSnapshots is not implemented for the File Storage driver")
}

func (c *controller) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ControllerExpandVolume is not implemented for the File Storage driver")
}
