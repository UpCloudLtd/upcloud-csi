package driver

import (
	"context"
	"fmt"
	"strings"

	"github.com/UpCloudLtd/upcloud-go-api/v4/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v4/upcloud/request"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func listSupportedCapabilities() []csi.ControllerServiceCapability_RPC_Type {
	return []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
	}
}

// CreateVolume provisions storage via UpCloud Storage service.
//nolint:funlen // Requires refactoring of request fields validation
func (d *Driver) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (resp *csi.CreateVolumeResponse, err error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Name cannot be empty")
	}

	if req.VolumeCapabilities == nil || len(req.VolumeCapabilities) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume VolumeCapabilities cannot be empty")
	}

	violations := validateCapabilities(req.VolumeCapabilities)
	if len(violations) > 0 {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("CreateVolume failed with the following violations: %s", strings.Join(violations, ", ")))
	}

	// determine the size of the storage
	storageSize, err := getStorageRange(req.CapacityRange)
	if err != nil {
		return nil, status.Error(codes.OutOfRange, fmt.Sprintf("CreateVolume failed to extract storage size: %s", err.Error()))
	}

	if req.AccessibilityRequirements != nil {
		for _, t := range req.AccessibilityRequirements.Requisite {
			region, ok := t.Segments["region"]
			if !ok {
				continue // nothing to do
			}

			if region != d.options.zone {
				return nil, status.Errorf(codes.ResourceExhausted, "volume can be only created in region: %q, got: %q", d.options.zone, region)
			}
		}
	}

	volumeName := req.Name

	log := d.log.WithFields(logrus.Fields{
		"volume_name":             volumeName,
		"storage_size_giga_bytes": storageSize / giB,
		"method":                  "create_volume",
		"volume_capabilities":     req.VolumeCapabilities,
	})
	log.Info("create volume called")

	// get volume first, and skip if exists
	volumes, err := d.upclouddriver.getStorageByName(ctx, volumeName)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if len(volumes) > 0 {
		if len(volumes) > 1 {
			return nil, fmt.Errorf("fatal: duplicate volume %q exists", volumeName)
		}
		vol := volumes[0].Storage

		if vol.Size*giB != int(storageSize) {
			return nil, status.Errorf(codes.AlreadyExists, fmt.Sprintf("invalid storage size requested: %d", storageSize))
		}

		log.Info("volume already exists")
		return &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:      vol.UUID,
				CapacityBytes: int64(vol.Size) * giB,
			},
		}, nil
	}

	tierMapper := map[string]string{"maxiops": upcloud.StorageTierMaxIOPS, "hdd": upcloud.StorageTierHDD}
	tier := tierMapper[req.Parameters["tier"]]
	volumeReq := &request.CreateStorageRequest{
		Zone:  d.options.zone,
		Title: volumeName,
		Size:  int(storageSize / giB),
		Tier:  tier,
	}

	log.WithField("volume_req", volumeReq).Info("creating volume")
	log.Debugf("volume request: %#v", *volumeReq)
	vol, err := d.upclouddriver.createStorage(ctx, volumeReq)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	volResp := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      vol.UUID,
			CapacityBytes: storageSize,
			AccessibleTopology: []*csi.Topology{
				{
					Segments: map[string]string{
						"region": d.options.zone,
					},
				},
			},
		},
	}

	log.WithField("response", resp).Info("volume created")

	return volResp, nil
}

// DeleteVolume deletes storage via UpCloud Storage service.
func (d *Driver) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "DeleteVolume Volume ID must be provided")
	}

	log := d.log.WithFields(logrus.Fields{
		"volume_id": req.VolumeId,
		"method":    "delete_volume",
	})
	log.Info("delete volume called")

	err := d.upclouddriver.deleteStorage(ctx, req.VolumeId)
	if err != nil {
		return nil, err
	}

	log.Info("volume was deleted")

	return &csi.DeleteVolumeResponse{}, nil
}

// ControllerPublishVolume attaches storage to a node via UpCloud Storage service.
//nolint:funlen // Requires refactoring of request fields validation
func (d *Driver) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume Volume ID must be provided")
	}

	if req.NodeId == "" {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume Node ID must be provided")
	}

	if req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume Volume capability must be provided")
	}

	if req.Readonly {
		return nil, status.Error(codes.AlreadyExists, "read only Volumes are not supported")
	}

	log := d.log.WithFields(logrus.Fields{
		"volume_id": req.VolumeId,
		"node_id":   req.NodeId,
		"method":    "controller_publish_volume",
	})
	log.Info("controller publish volume called")

	// check if volume exist before trying to attach it
	volumes, err := d.upclouddriver.getStorageByUUID(ctx, req.VolumeId)
	if err != nil {
		d.log.Errorf("get storage by uuid error: %s, %#v", err, volumes)
		return nil, err
	}

	if len(volumes) == 0 {
		return nil, fmt.Errorf("volume doesn't exist")
	} else if len(volumes) > 1 {
		return nil, fmt.Errorf("too many volumes")
	}
	volume := volumes[0]

	// TODO
	/*if volume.State == "maintenance" {
	}*/

	attachedID := ""
	for _, id := range volume.ServerUUIDs {
		attachedID = id
		if id == req.NodeId {
			log.Info("volume is already attached")
			return &csi.ControllerPublishVolumeResponse{
				PublishContext: map[string]string{
					d.options.volumeName: volume.Title,
				},
			}, nil
		}
	}

	// volume is attached to a different node, return an error
	if attachedID != "" {
		return nil, status.Errorf(codes.FailedPrecondition,
			"volume %q is attached to the wrong node (%s), detach the volume to fix it",
			req.VolumeId, attachedID)
	}

	// attach the volume to the correct node
	server, err := d.upclouddriver.getServerByHostname(ctx, req.NodeId)
	if err != nil {
		return nil, err
	}

	err = d.upclouddriver.attachStorage(ctx, req.VolumeId, server.UUID)
	if err != nil {
		// already attached to the node
		return nil, err
	}

	log.Info("volume was attached")
	return &csi.ControllerPublishVolumeResponse{
		PublishContext: map[string]string{
			d.options.volumeName: volume.Title,
		},
	}, nil
}

// ControllerUnpublishVolume detaches storage from a node via UpCloud Storage service.
func (d *Driver) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "ControllerUnpublishVolume Volume ID must be provided")
	}

	log := d.log.WithFields(logrus.Fields{
		"volume_id": req.VolumeId,
		"node_id":   req.NodeId,
		"method":    "controller_unpublish_volume",
	})
	log.Info("controller unpublish volume called")

	// check if volume exist before trying to detach it
	_, err := d.upclouddriver.getStorageByUUID(ctx, req.VolumeId)
	if err != nil {
		return nil, err
	}

	server, err := d.upclouddriver.getServerByHostname(ctx, req.NodeId)
	if err != nil {
		return nil, err
	}

	err = d.upclouddriver.detachStorage(ctx, req.VolumeId, server.UUID)
	if err != nil {
		return nil, err
	}

	log.Info("volume was detached")
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

// ValidateVolumeCapabilities checks if the volume capabilities are valid.
func (d *Driver) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "ValidateVolumeCapabilities Volume ID must be provided")
	}

	if req.VolumeCapabilities == nil {
		return nil, status.Error(codes.InvalidArgument, "ValidateVolumeCapabilities Volume Capabilities must be provided")
	}

	log := d.log.WithFields(logrus.Fields{
		"volume_id":              req.VolumeId,
		"volume_capabilities":    req.VolumeCapabilities,
		"supported_capabilities": GetDefaultAccessMode(),
		"method":                 "validate_volume_capabilities",
	})
	log.Info("validate volume capabilities called")

	// check if volume exist before trying to validate it
	_, err := d.upclouddriver.getStorageByUUID(ctx, req.VolumeId)
	if err != nil {
		return nil, err
	}

	// if it's not supported (i.e: wrong region), we shouldn't override it
	resp := &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: []*csi.VolumeCapability{
				{
					AccessMode: GetDefaultAccessMode(),
				},
			},
		},
	}

	log.WithField("confirmed", resp.Confirmed).Info("supported capabilities")
	return resp, nil
}

// ListVolumes returns a list of all requested volumes.
func (d *Driver) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	log := d.log.WithFields(logrus.Fields{
		"max_entries":        req.MaxEntries,
		"req_starting_token": req.StartingToken,
		"method":             "list_volumes",
	})
	log.Info("list volumes called")

	// TODO OPTIONAL: implement starting token / pagination
	//var startingToken int32
	//if req.StartingToken != "" {
	//	parsedToken, err := strconv.ParseInt(req.StartingToken, 10, 32)
	//	if err != nil {
	//		return nil, status.Errorf(codes.Aborted, "ListVolumes starting token %q is not valid: %s", req.StartingToken, err)
	//	}
	//	startingToken = int32(parsedToken)
	//}

	volumes, err := d.upclouddriver.listStorage(ctx, d.options.zone)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "listvolumes failed with: %s", err.Error())
	}

	entries := make([]*csi.ListVolumesResponse_Entry, len(volumes), len(volumes))
	for i, vol := range volumes {
		entries[i] = &csi.ListVolumesResponse_Entry{
			Volume: &csi.Volume{
				VolumeId:      vol.UUID,
				CapacityBytes: int64(vol.Size) * giB,
			},
		}
	}

	resp := &csi.ListVolumesResponse{
		Entries: entries,
	}

	// TODO start token
	//if nextToken > 0 {
	//	resp.NextToken = strconv.FormatInt(int64(nextToken), 10)
	//}

	log.WithField("response", resp).Info("volumes listed")
	return resp, nil
}

// GetCapacity returns the capacity of the storage pool.
func (d *Driver) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	// TODO
	d.log.WithFields(logrus.Fields{
		"params": req.Parameters,
		"method": "get_capacity",
	}).Warn("get capacity is not implemented")

	return nil, status.Error(codes.Unimplemented, "")
}

// ControllerGetCapabilities returns the capacity of the storage pool.
func (d *Driver) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	newCap := func(cap_ csi.ControllerServiceCapability_RPC_Type) *csi.ControllerServiceCapability {
		return &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: cap_,
				},
			},
		}
	}

	supportedCaps := listSupportedCapabilities()
	caps := make([]*csi.ControllerServiceCapability, len(supportedCaps), len(supportedCaps))
	for i, capability := range supportedCaps {
		caps[i] = newCap(capability)
	}

	resp := &csi.ControllerGetCapabilitiesResponse{
		Capabilities: caps,
	}

	d.log.WithFields(logrus.Fields{
		"response": resp,
		"method":   "controller_get_capabilities",
	}).Info("controller get capabilities called")

	return resp, nil
}

// CreateSnapshot will be called by the CO to create a new snapshot from a
// source volume on behalf of a user.
func (d *Driver) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	// TODO
	return nil, status.Error(codes.Unimplemented, "")
}

// DeleteSnapshot will be called by the CO to delete a snapshot.
func (d *Driver) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	// TODO
	return nil, status.Error(codes.Unimplemented, "")
}

// ListSnapshots returns the information about all snapshots on the storage
// system within the given parameters regardless of how they were created.
// ListSnapshots shold not list a snapshot that is being created but has not
// been cut successfully yet.
func (d *Driver) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	// TODO
	return nil, status.Error(codes.Unimplemented, "")
}

// ControllerExpandVolume is called from the resizer to increase the volume size.
//nolint:funlen // Requires refactoring of request fields validation
func (d *Driver) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	volumeID := req.VolumeId

	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "ControllerExpandVolume volume ID missing in request")
	}

	volumes, err := d.upclouddriver.getStorageByUUID(ctx, volumeID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "ControllerExpandVolume could not retrieve existing volumes: %v", err)
	}

	if len(volumes) == 0 {
		return nil, fmt.Errorf("volume doesn't exist")
	} else if len(volumes) > 1 {
		return nil, fmt.Errorf("too many volumes")
	}
	volume := volumes[0]

	resizeBytes, err := getStorageRange(req.CapacityRange)
	if err != nil {
		return nil, status.Errorf(codes.OutOfRange, "ControllerExpandVolume invalid capacity range: %v", err)
	}
	resizeGigaBytes := resizeBytes / giB

	log := d.log.WithFields(logrus.Fields{
		"volume_id": volumeID,
		"method":    "controller_expand_volume",
	})

	log.Infof("controller expand volume called: volume - %v", volumes)

	if resizeGigaBytes <= int64(volume.Size) {
		log.WithFields(logrus.Fields{
			"current_volume_size":   volume.Size,
			"requested_volume_size": resizeGigaBytes,
		}).Info("skipping volume resizeStorage because current volume size exceeds requested volume size")
		return &csi.ControllerExpandVolumeResponse{CapacityBytes: int64(volume.Size * giB), NodeExpansionRequired: true}, nil
	}

	if len(volume.ServerUUIDs) == 0 {
		return nil, fmt.Errorf("volume is not attached to any server")
	}

	nodeID := volume.ServerUUIDs[0]
	err = d.upclouddriver.detachStorage(ctx, volumeID, nodeID)
	if err != nil {
		return nil, err
	}

	d.log.WithFields(logrus.Fields{
		"volume_id": volumeID,
		"node_id":   nodeID,
	}).Info("volume detached")

	_, err = d.upclouddriver.resizeStorage(ctx, volume.UUID, int(resizeGigaBytes))
	if err != nil {
		d.log.Errorf("cannot resizeStorage volume %s: %s", volumeID, err.Error())
	}

	err = d.upclouddriver.attachStorage(ctx, volumeID, nodeID)
	if err != nil {
		return nil, err
	}

	d.log.WithFields(logrus.Fields{
		"volume_id": volumeID,
		"node_id":   nodeID,
	}).Info("volume attached")

	log = log.WithField("new_volume_size", resizeGigaBytes)
	log.Info("volume was resized")

	nodeExpansionRequired := true
	if req.VolumeCapability != nil {
		if _, ok := req.VolumeCapability.AccessType.(*csi.VolumeCapability_Block); ok {
			log.Info("nodeID expansion is not required for block volumes")
			nodeExpansionRequired = false
		}
	}

	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         resizeGigaBytes * giB,
		NodeExpansionRequired: nodeExpansionRequired,
	}, nil
}

func (d *Driver) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ControllerGetVolume is unimplemented")
}
