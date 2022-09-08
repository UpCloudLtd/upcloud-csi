package driver

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/UpCloudLtd/upcloud-go-api/v4/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v4/upcloud/request"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var supportedCapabilities = []csi.ControllerServiceCapability_RPC_Type{
	csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
	csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
	csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
	csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
	csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
	csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
	csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
}

// CreateVolume provisions storage via UpCloud Storage service
func (d *Driver) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (resp *csi.CreateVolumeResponse, err error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Name cannot be empty")
	}

	if req.VolumeCapabilities == nil || len(req.VolumeCapabilities) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume VolumeCapabilities cannot be empty")
	}

	violations := validateCapabilities(req.VolumeCapabilities)
	if violations != nil && len(violations) > 0 {
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
			return nil, status.Errorf(codes.AlreadyExists, "invalid storage size requested: %d", storageSize)
		}

		log.Info("volume already exists")
		return &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:      vol.UUID,
				CapacityBytes: int64(vol.Size) * giB,
				ContentSource: req.GetVolumeContentSource(),
			},
		}, nil
	}

	tierMapper := map[string]string{"maxiops": upcloud.StorageTierMaxIOPS, "hdd": upcloud.StorageTierHDD}
	tier := tierMapper[req.Parameters["tier"]]
	var vol *upcloud.StorageDetails
	storageSizeGB := int(storageSize / giB)

	if volContentSrc := req.GetVolumeContentSource(); volContentSrc != nil {
		var sourceID string
		switch volContentSrc.Type.(type) {
		case *csi.VolumeContentSource_Snapshot:
			snapshot := volContentSrc.GetSnapshot()
			if snapshot == nil {
				return nil, status.Error(codes.Internal, "content source snapshot is not defined")
			}
			sourceID = snapshot.GetSnapshotId()
		case *csi.VolumeContentSource_Volume:
			srcVol := volContentSrc.GetVolume()
			if srcVol == nil {
				return nil, status.Error(codes.Internal, "content source volume is not defined")
			}
			sourceID = srcVol.GetVolumeId()
		default:
			return nil, status.Errorf(codes.InvalidArgument, "%v not a proper volume source", volContentSrc)
		}
		src, err := d.upclouddriver.getStorageByUUID(ctx, sourceID)
		if err != nil {
			return nil, fmt.Errorf("could not retrieve source volume by ID: %v", err)
		}

		volumeReq := &request.CloneStorageRequest{
			UUID:  src.Storage.UUID,
			Zone:  d.options.zone,
			Tier:  tier,
			Title: volumeName,
		}
		log.WithField("volume_req", volumeReq).Infof("creating volume by cloning storage ID %s", src.Storage.UUID)
		vol, err = d.upclouddriver.cloneStorage(ctx, volumeReq)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		if storageSizeGB > vol.Storage.Size {
			log.WithField("volume_req", volumeReq).Infof("resizing created volume %s from %dGB to %dGB", vol.Storage.UUID, vol.Storage.Size, storageSizeGB)
			// resize cloned storage and delete backup taken during resize operation as this is newly created storage
			if vol, err = d.upclouddriver.resizeStorage(ctx, vol.Storage.UUID, storageSizeGB, true); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		}
	} else {
		volumeReq := &request.CreateStorageRequest{
			Zone:  d.options.zone,
			Title: volumeName,
			Size:  storageSizeGB,
			Tier:  tier,
		}

		log.WithField("volume_req", volumeReq).Info("creating volume")
		log.Debugf("volume request: %#v", *volumeReq)
		if vol, err = d.upclouddriver.createStorage(ctx, volumeReq); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
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
			ContentSource: req.GetVolumeContentSource(),
		},
	}

	log.WithField("response", resp).Info("volume created")

	return volResp, nil
}

// DeleteVolume deletes storage via UpCloud Storage service
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

// ControllerPublishVolume attaches storage to a node via UpCloud Storage service
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

	server, err := d.upclouddriver.getServerByHostname(ctx, req.NodeId)
	if err != nil {
		return nil, err
	}

	log := d.log.WithFields(logrus.Fields{
		"volume_id": req.VolumeId,
		"node_id":   req.NodeId,
		"method":    "controller_publish_volume",
	})
	log.Info("controller publish volume called")

	// check if volume exist before trying to attach it
	volume, err := d.upclouddriver.getStorageByUUID(ctx, req.VolumeId)
	if err != nil {
		d.log.Errorf("get storage by uuid error: %s", err)
		return nil, err
	}

	if volume.State == "maintenance" {
		// TODO
	}

	attachedID := ""
	for _, id := range volume.ServerUUIDs {
		attachedID = id
		if id == server.UUID {
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

// ControllerUnpublishVolume detaches storage from a node via UpCloud Storage service
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

func (d *Driver) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	// ALPHA FEATURE
	// This optional RPC MAY be called by the CO to fetch current information about a volume.
	// A Controller Plugin MUST implement this ControllerGetVolume RPC call if it has GET_VOLUME capability.
	// When implemented add csi.ControllerServiceCapability_RPC_GET_VOLUME to supportedCapabilities.
	return nil, status.Errorf(codes.Unimplemented, "method ControllerGetVolume not implemented")
}

// ValidateVolumeCapabilities checks if the volume capabilities are valid
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
		"supported_capabilities": supportedAccessMode,
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
					AccessMode: supportedAccessMode,
				},
			},
		},
	}

	log.WithField("confirmed", resp.Confirmed).Info("supported capabilities")
	return resp, nil
}

// ListVolumes returns a list of all requested volumes
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

	entries := make([]*csi.ListVolumesResponse_Entry, 0)
	for _, vol := range volumes {
		entries = append(entries, &csi.ListVolumesResponse_Entry{
			Volume: &csi.Volume{
				VolumeId:      vol.UUID,
				CapacityBytes: int64(vol.Size) * giB,
			},
		})
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

// GetCapacity returns the capacity of the storage pool
func (d *Driver) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	// TODO
	d.log.WithFields(logrus.Fields{
		"params": req.Parameters,
		"method": "get_capacity",
	}).Warn("get capacity is not implemented")

	return nil, status.Error(codes.Unimplemented, "")
}

// ControllerGetCapabilities returns the capacity of the storage pool
func (d *Driver) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	newCap := func(cap csi.ControllerServiceCapability_RPC_Type) *csi.ControllerServiceCapability {
		return &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: cap,
				},
			},
		}
	}

	var caps []*csi.ControllerServiceCapability
	for _, capability := range supportedCapabilities {
		caps = append(caps, newCap(capability))
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
	s, err := d.upclouddriver.getStorageBackupByName(ctx, req.GetName())
	if err != nil && err != errUpCloudStorageNotFound {
		return nil, status.Errorf(codes.Internal, "createsnapshot failed with: %s", err.Error())
	}

	if s == nil {
		sd, err := d.upclouddriver.createStorageBackup(ctx, req.GetSourceVolumeId(), req.GetName())
		if err != nil {
			return nil, status.Errorf(codes.Internal, "createsnapshot failed with: %s", err.Error())
		}
		s = &sd.Storage
	}

	return &csi.CreateSnapshotResponse{
		Snapshot: &csi.Snapshot{
			SizeBytes:      int64(s.Size) * giB,
			SnapshotId:     s.UUID,
			SourceVolumeId: s.Origin,
			CreationTime:   timestamppb.New(s.Created),
			ReadyToUse:     s.State == upcloud.StorageStateOnline,
		},
	}, nil
}

// DeleteSnapshot will be called by the CO to delete a snapshot.
func (d *Driver) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	if err := d.upclouddriver.deleteStorageBackup(ctx, req.GetSnapshotId()); err != nil {
		return nil, status.Errorf(codes.Internal, "deletesnapshot failed with: %s", err.Error())
	}

	return &csi.DeleteSnapshotResponse{}, nil
}

// ListSnapshots returns the information about all snapshots on the storage
// system within the given parameters regardless of how they were created.
// ListSnapshots should not list a snapshot that is being created but has not
// been cut successfully yet.
func (d *Driver) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	backups, err := d.upclouddriver.listStorageBackups(ctx, req.GetSourceVolumeId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "listsnapshots failed with: %s", err.Error())
	}
	entries := make([]*csi.ListSnapshotsResponse_Entry, 0)
	for _, s := range backups {
		entries = append(entries, &csi.ListSnapshotsResponse_Entry{
			Snapshot: &csi.Snapshot{
				SizeBytes:      int64(s.Size) * giB,
				SnapshotId:     s.UUID,
				SourceVolumeId: s.Origin,
				CreationTime:   timestamppb.New(s.Created),
				ReadyToUse:     s.State == upcloud.StorageStateOnline,
			},
		})
	}
	// TODO OPTIONAL: implement starting token / pagination
	return &csi.ListSnapshotsResponse{
		Entries: entries,
	}, nil
}

// ControllerExpandVolume is called from the resizer to increase the volume size.
func (d *Driver) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	volumeId := req.VolumeId

	if len(volumeId) == 0 {
		return nil, status.Error(codes.InvalidArgument, "ControllerExpandVolume volume ID missing in request")
	}

	volume, err := d.upclouddriver.getStorageByUUID(ctx, volumeId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "ControllerExpandVolume could not retrieve existing volumes: %v", err)
	}

	resizeBytes, err := obtainSize(req.CapacityRange)
	if err != nil {
		return nil, status.Errorf(codes.OutOfRange, "ControllerExpandVolume invalid capacity range: %v", err)
	}
	resizeGigaBytes := resizeBytes / giB

	log := d.log.WithFields(logrus.Fields{
		"volume_id": volumeId,
		"method":    "controller_expand_volume",
	})

	log.Infof("controller expand volume called: volume - %v", volume)

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

	nodeId := volume.ServerUUIDs[0]
	err = d.upclouddriver.detachStorage(ctx, volumeId, nodeId)
	if err != nil {
		return nil, err
	}

	d.log.WithFields(logrus.Fields{
		"volume_id": volumeId,
		"node_id":   nodeId,
	}).Info("volume detached")

	_, err = d.upclouddriver.resizeStorage(ctx, volume.UUID, int(resizeGigaBytes), false)
	if err != nil {
		d.log.Errorf("cannot resizeStorage volume %s: %s", volumeId, err.Error())
	}

	err = d.upclouddriver.attachStorage(ctx, volumeId, nodeId)
	if err != nil {
		return nil, err
	}

	d.log.WithFields(logrus.Fields{
		"volume_id": volumeId,
		"node_id":   nodeId,
	}).Info("volume attached")

	log = log.WithField("new_volume_size", resizeGigaBytes)

	//if resizedStorage != nil {
	//	log.Info("waiting until volumes is resized")
	//	if err := d.waitAction(ctx, log, volumeId, resizedStorage.ID); err != nil {
	//		return nil, status.Errorf(codes.Internal, "failed waiting for volumes to get resized: %s", err)
	//	}
	//}

	log.Info("volume was resized")

	nodeExpansionRequired := true
	if req.VolumeCapability != nil {
		if _, ok := req.VolumeCapability.AccessType.(*csi.VolumeCapability_Block); ok {
			log.Info("nodeId expansion is not required for block volumes")
			nodeExpansionRequired = false
		}
	}

	return &csi.ControllerExpandVolumeResponse{CapacityBytes: resizeGigaBytes * giB, NodeExpansionRequired: nodeExpansionRequired}, nil
}

func obtainSize(capRange *csi.CapacityRange) (int64, error) {
	if capRange == nil {
		return defaultVolumeSize, nil
	}

	requiredBytes := capRange.GetRequiredBytes()
	requiredSet := 0 < requiredBytes
	limitBytes := capRange.GetLimitBytes()
	limitSet := 0 < limitBytes

	if !requiredSet && !limitSet {
		return defaultVolumeSize, nil
	}

	if requiredSet && limitSet && limitBytes < requiredBytes {
		return 0, fmt.Errorf("limit (%v) can not be less than required (%v) size", formatBytes(limitBytes), formatBytes(requiredBytes))
	}

	if requiredSet && !limitSet && requiredBytes < minimumVolumeSizeInBytes {
		return 0, fmt.Errorf("required (%v) can not be less than minimum supported volume size (%v)", formatBytes(requiredBytes), formatBytes(minimumVolumeSizeInBytes))
	}

	if limitSet && limitBytes < minimumVolumeSizeInBytes {
		return 0, fmt.Errorf("limit (%v) can not be less than minimum supported volume size (%v)", formatBytes(limitBytes), formatBytes(minimumVolumeSizeInBytes))
	}

	if requiredSet && requiredBytes > maximumVolumeSizeInBytes {
		return 0, fmt.Errorf("required (%v) can not exceed maximum supported volume size (%v)", formatBytes(requiredBytes), formatBytes(maximumVolumeSizeInBytes))
	}

	if !requiredSet && limitSet && limitBytes > maximumVolumeSizeInBytes {
		return 0, fmt.Errorf("limit (%v) can not exceed maximum supported volume size (%v)", formatBytes(limitBytes), formatBytes(maximumVolumeSizeInBytes))
	}

	if requiredSet && limitSet && requiredBytes == limitBytes {
		return requiredBytes, nil
	}

	if requiredSet {
		return requiredBytes, nil
	}

	if limitSet {
		return limitBytes, nil
	}

	return defaultVolumeSize, nil
}

func formatBytes(inputBytes int64) string {
	output := float64(inputBytes)
	unit := ""

	switch {
	case inputBytes >= tiB:
		output = output / tiB
		unit = "Tb"
	case inputBytes >= giB:
		output = output / giB
		unit = "Gb"
	case inputBytes >= miB:
		output = output / miB
		unit = "Mb"
	case inputBytes >= kiB:
		output = output / kiB
		unit = "Kb"
	case inputBytes == 0:
		return "0"
	}

	result := strconv.FormatFloat(output, 'f', 1, 64)
	result = strings.TrimSuffix(result, ".0")

	return result + unit
}
