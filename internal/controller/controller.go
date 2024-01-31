package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/UpCloudLtd/upcloud-csi/internal/logger"
	"github.com/UpCloudLtd/upcloud-csi/internal/service"
	"github.com/UpCloudLtd/upcloud-go-api/v6/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v6/upcloud/request"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var supportedCapabilities = []csi.ControllerServiceCapability_RPC_Type{ //nolint: gochecknoglobals // readonly variable
	csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
	csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
	csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
	csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
	csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
	csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
	csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
}

type Controller struct {
	zone              string
	maxVolumesPerNode int

	svc service.Service
	log *logrus.Entry

	storageLabels []upcloud.Label
}

func NewController(svc service.Service, zone string, maxVolumesPerNode int, l *logrus.Entry, labels ...string) (*Controller, error) {
	if zone == "" {
		return nil, errors.New("controller zone is required field")
	}
	return &Controller{
		zone:              zone,
		svc:               svc,
		log:               l,
		storageLabels:     upcloudLabels(labels),
		maxVolumesPerNode: maxVolumesPerNode,
	}, nil
}

// CreateVolume provisions storage via UpCloud Storage service.
func (c *Controller) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (resp *csi.CreateVolumeResponse, err error) {
	log := logger.WithServerContext(ctx, c.log).WithField(logger.VolumeNameKey, req.GetName())

	if err := validateCreateVolumeRequest(req, c.zone); err != nil {
		return nil, err
	}
	// get volume first, and skip if exists
	volumes, err := c.svc.GetStorageByName(ctx, req.GetName())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if len(volumes) > 0 {
		return createVolumeExistsResponse(ctx, req, volumes, log)
	}

	tier, err := createVolumeRequestTier(req)
	if err != nil {
		return nil, err
	}
	// determine the size of the storage
	storageSize, err := getStorageRange(req.GetCapacityRange())
	if err != nil {
		return nil, status.Error(codes.OutOfRange, fmt.Sprintf("CreateVolume failed to extract storage size: %s", err.Error()))
	}
	storageSizeGB := int(storageSize / giB)

	var vol *upcloud.StorageDetails
	if volContentSrc := req.GetVolumeContentSource(); volContentSrc != nil {
		if vol, err = c.createVolumeFromSource(ctx, req, storageSizeGB, tier); err != nil {
			return nil, err
		}
	} else {
		volumeReq := &request.CreateStorageRequest{
			Zone:      c.zone,
			Title:     req.GetName(),
			Size:      storageSizeGB,
			Tier:      tier,
			Labels:    c.storageLabels,
			Encrypted: upcloud.FromBool(createVolumeRequestEncryptionAtRest(req)),
		}
		logger.WithServiceRequest(log, volumeReq).Info("creating volume")
		if vol, err = c.svc.CreateStorage(ctx, volumeReq); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      vol.UUID,
			CapacityBytes: storageSize,
			AccessibleTopology: []*csi.Topology{
				{
					Segments: map[string]string{
						"region": c.zone,
					},
				},
			},
			ContentSource: req.GetVolumeContentSource(),
		},
	}, nil
}

func createVolumeExistsResponse(ctx context.Context, req *csi.CreateVolumeRequest, volumes []*upcloud.StorageDetails, log *logrus.Entry) (resp *csi.CreateVolumeResponse, err error) {
	if len(volumes) > 1 {
		return nil, fmt.Errorf("fatal: duplicate volume %q exists", req.GetName())
	}
	vol := volumes[0].Storage
	storageSize, err := getStorageRange(req.GetCapacityRange())
	if err != nil {
		return nil, status.Error(codes.OutOfRange, fmt.Sprintf("CreateVolume failed to extract storage size: %s", err.Error()))
	}
	if vol.Size*giB != int(storageSize) {
		return nil, status.Errorf(codes.AlreadyExists, "invalid storage size requested: %d", storageSize)
	}
	log.WithField(logger.VolumeIDKey, vol.UUID).Info("volume already exists")
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      vol.UUID,
			CapacityBytes: int64(vol.Size) * giB,
			ContentSource: req.GetVolumeContentSource(),
		},
	}, nil
}

func (c *Controller) createVolumeFromSource(ctx context.Context, req *csi.CreateVolumeRequest, storageSizeGB int, tier string) (*upcloud.StorageDetails, error) {
	volContentSrc := req.GetVolumeContentSource()
	if volContentSrc == nil {
		return nil, status.Error(codes.Internal, "got empty volume content source")
	}
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
	log := logger.WithServerContext(ctx, c.log).WithField(logger.VolumeNameKey, req.GetName()).WithField(logger.VolumeSourceKey, sourceID)
	log.Info("getting source storage by uuid")
	src, err := c.svc.GetStorageByUUID(ctx, sourceID)
	if err != nil {
		if errors.Is(err, service.ErrStorageNotFound) {
			return nil, status.Errorf(codes.NotFound, "could not retrieve source volume by ID: %s", err.Error())
		}
		return nil, status.Errorf(codes.InvalidArgument, err.Error())
	}
	if src.Encrypted.Bool() != createVolumeRequestEncryptionAtRest(req) {
		// To prevent unexpectected dst device properties, only allow cloning from device with same encryption policy.
		return nil, status.Errorf(codes.InvalidArgument, "source and destination volumes needs to have same encryption policy")
	}
	log.Info("checking that source storage is online")
	if err := c.svc.RequireStorageOnline(ctx, &src.Storage); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	volumeReq := &request.CloneStorageRequest{
		UUID:      src.Storage.UUID,
		Zone:      c.zone,
		Tier:      tier,
		Title:     req.GetName(),
		Encrypted: src.Encrypted,
	}
	logger.WithServiceRequest(log, volumeReq).Info("cloning volume")
	vol, err := c.svc.CloneStorage(ctx, volumeReq, c.storageLabels...)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	log = log.WithField(logger.VolumeIDKey, vol.Storage.UUID).WithField("size", vol.Storage.Size)
	if storageSizeGB > vol.Storage.Size {
		log.WithField("new_size", storageSizeGB).Info("resizing volume")
		// resize cloned storage and delete backup taken during resize operation as this is newly created storage
		if vol, err = c.svc.ResizeStorage(ctx, vol.Storage.UUID, storageSizeGB, true); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	return vol, err
}

// DeleteVolume deletes storage via UpCloud Storage service.
func (c *Controller) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "DeleteVolume Volume ID must be provided")
	}

	logger.WithServerContext(ctx, c.log).WithField(logger.VolumeIDKey, req.GetVolumeId()).Info("deleting volume")
	err := c.svc.DeleteStorage(ctx, req.VolumeId)
	if err != nil && !errors.Is(err, service.ErrStorageNotFound) {
		return &csi.DeleteVolumeResponse{}, err
	}
	return &csi.DeleteVolumeResponse{}, nil
}

// ControllerPublishVolume attaches storage to a node via UpCloud Storage service.
func (c *Controller) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) { //nolint: funlen // TODO: refactor
	if err := validateControllerPublishVolumeRequest(req); err != nil {
		return nil, err
	}
	log := logger.WithServerContext(ctx, c.log).WithField(logger.VolumeIDKey, req.GetVolumeId()).WithField(logger.NodeIDKey, req.GetNodeId())

	server, err := c.svc.GetServerByHostname(ctx, req.NodeId)
	if err != nil {
		if errors.Is(err, service.ErrServerNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// check if volume exist before trying to attach it
	log.Info("getting storage by uuid")
	volume, err := c.svc.GetStorageByUUID(ctx, req.VolumeId)
	if err != nil {
		if errors.Is(err, service.ErrStorageNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	log.Info("checking that storage is online")
	if err = c.svc.RequireStorageOnline(ctx, &volume.Storage); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	attachedID := ""
	for _, id := range volume.ServerUUIDs {
		attachedID = id
		if id == server.UUID {
			log.Info("volume is already attached")
			return &csi.ControllerPublishVolumeResponse{
				PublishContext: map[string]string{
					string(logger.CtxCorrelationIDKey): logger.ContextCorrelationID(ctx),
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

	log.Info("check that volumes already attached to the node is less than the maximum supported")
	// Slice server.StorageDevices contains at least one additional root disk device
	// so if len(server.StorageDevices) is equal to maxVolumesPerNode there is still room for one device.
	// At the moment there is no reliable way to tell which devices are managed by CSI and which are e.g. additional devices created by user.
	if len(server.StorageDevices) > c.maxVolumesPerNode {
		return nil, status.Error(codes.ResourceExhausted, "volumes already attached to the node is more than the maximum supported")
	}
	log.Info("attaching storage to node")
	err = c.svc.AttachStorage(ctx, req.VolumeId, server.UUID)
	if err != nil {
		var svcError *upcloud.Problem
		if errors.As(err, &svcError) && svcError.Status != http.StatusConflict && svcError.ErrorCode() == upcloud.ErrCodeStorageDeviceLimitReached {
			return nil, status.Error(codes.ResourceExhausted, "The limit of the number of attached devices has been reached")
		}
		return nil, err
	}

	return &csi.ControllerPublishVolumeResponse{
		PublishContext: map[string]string{
			string(logger.CtxCorrelationIDKey): logger.ContextCorrelationID(ctx),
		},
	}, nil
}

// ControllerUnpublishVolume is a reverse operation of ControllerPublishVolume.
//
// The Plugin SHOULD perform the work that is necessary for making the volume ready to be consumed by a different node.
// The Plugin MUST NOT assume that this RPC will be executed on the node where the volume was previously used.
//
// This RPC is typically called by the CO when the workload using the volume is being moved to a different node,
// or all the workload using the volume on a node has finished.
//
// If the volume corresponding to the volume_id or the node corresponding to node_id cannot be found by the Plugin
// and the volume can be safely regarded as ControllerUnpublished from the node, the plugin SHOULD return 0 OK.
func (c *Controller) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID must be provided")
	}
	log := logger.WithServerContext(ctx, c.log).WithFields(logrus.Fields{
		logger.VolumeIDKey: req.GetVolumeId(),
		logger.NodeIDKey:   req.GetNodeId(),
	})
	log.Info("getting storage by uuid")
	// check if volume exist before trying to detach it
	_, err := c.svc.GetStorageByUUID(ctx, req.GetVolumeId())
	if err != nil {
		if errors.Is(err, service.ErrStorageNotFound) {
			log.Info("storage not found")
			return &csi.ControllerUnpublishVolumeResponse{}, nil
		}
		return nil, err
	}

	// TODO:  If node ID is not set, the SP MUST unpublish the volume from all nodes it is published to (ref. ControllerUnpublishVolumeRequest.NodeId).
	log.Info("getting server by hostname")
	server, err := c.svc.GetServerByHostname(ctx, req.GetNodeId())
	if err != nil {
		if errors.Is(err, service.ErrServerNotFound) {
			log.Info("server not found")
			return &csi.ControllerUnpublishVolumeResponse{}, nil
		}
		return nil, err
	}

	log.Info("detaching volume")
	err = c.svc.DetachStorage(ctx, req.VolumeId, server.UUID)
	if err != nil {
		if errors.Is(err, service.ErrServerStorageNotFound) {
			log.Info("volume was already detached from the node")
			return &csi.ControllerUnpublishVolumeResponse{}, nil
		}
		return nil, err
	}

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (c *Controller) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	// ALPHA FEATURE
	// This optional RPC MAY be called by the CO to fetch current information about a volume.
	// A Controller Plugin MUST implement this ControllerGetVolume RPC call if it has GET_VOLUME capability.
	// When implemented add csi.ControllerServiceCapability_RPC_GET_VOLUME to supportedCapabilities.
	return nil, status.Errorf(codes.Unimplemented, "method ControllerGetVolume not implemented")
}

// ValidateVolumeCapabilities checks if the volume capabilities are valid.
func (c *Controller) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID must be provided")
	}
	log := logger.WithServerContext(ctx, c.log).WithField(logger.VolumeIDKey, req.GetVolumeId())

	if req.VolumeCapabilities == nil {
		return nil, status.Error(codes.InvalidArgument, "volume vapabilities must be provided")
	}

	log.Info("getting storage by uuid")
	// check if volume exist before trying to validate it
	if _, err := c.svc.GetStorageByUUID(ctx, req.VolumeId); err != nil {
		if errors.Is(err, service.ErrStorageNotFound) {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
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

// ListVolumes returns a list of all requested volumes.
// TODO OPTIONAL: implement starting token / pagination.
func (c *Controller) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	log := logger.WithServerContext(ctx, c.log).WithFields(logrus.Fields{
		logger.ListStartingTokenKey: req.GetStartingToken(),
		logger.ListMaxEntriesKey:    req.GetMaxEntries(),
	})
	listStart, err := parseToken(req.GetStartingToken())
	if err != nil {
		return nil, status.Error(codes.Aborted, "failed to parse starting_token")
	}
	log.Info("getting list of storages")
	volumes, err := c.svc.ListStorage(ctx, c.zone)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "listvolumes failed with: %s", err.Error())
	}

	volumes, listNext := paginateStorage(volumes, listStart, int(req.GetMaxEntries()))

	entries := make([]*csi.ListVolumesResponse_Entry, 0)
	for _, vol := range volumes {
		entries = append(entries, &csi.ListVolumesResponse_Entry{
			Volume: &csi.Volume{
				VolumeId:      vol.UUID,
				CapacityBytes: int64(vol.Size) * giB,
			},
		})
	}

	log.Infof("found %d storages", len(entries))
	return &csi.ListVolumesResponse{
		Entries:   entries,
		NextToken: fmt.Sprint(listNext),
	}, nil
}

// GetCapacity returns the capacity of the storage pool.
func (c *Controller) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// ControllerGetCapabilities returns the capacity of the storage pool.
func (c *Controller) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
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

	logger.WithServerContext(ctx, c.log).WithField("caps", caps).Info("reporting capabilities")
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: caps,
	}, nil
}

// CreateSnapshot will be called by the CO to create a new snapshot from a
// source volume on behalf of a user.
func (c *Controller) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "snapshot name must be provided")
	}
	if req.GetSourceVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "snapshot source volume ID must be provided")
	}

	log := logger.WithServerContext(ctx, c.log)
	log.Info("getting storage backup by name")

	s, err := c.svc.GetStorageBackupByName(ctx, req.GetName())
	if err != nil && !errors.Is(err, service.ErrStorageNotFound) {
		return nil, status.Errorf(codes.Internal, "CreateSnapshot failed with: %s", err.Error())
	}

	if s != nil && s.Origin != req.GetSourceVolumeId() {
		return nil, status.Error(codes.AlreadyExists, "snapshot already exists with different source volume ID")
	}

	if s == nil {
		log.Info("creating storage backup")

		sd, err := c.svc.CreateStorageBackup(ctx, req.GetSourceVolumeId(), req.GetName())
		if err != nil {
			if errors.Is(err, service.ErrBackupInProgress) {
				return nil, status.Errorf(codes.Aborted, "cannot create snapshot for volume with backup in progress")
			}

			return nil, status.Errorf(codes.Internal, "CreateSnapshot failed with: %s", err.Error())
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
func (c *Controller) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	snapID := req.GetSnapshotId()
	if snapID == "" {
		return nil, status.Error(codes.InvalidArgument, "snapshot ID must be provided")
	}
	// Delete should succeed if snapshot is not found or an invalid snapshot id is used.
	if isValidStorageUUID(snapID) {
		if err := c.svc.DeleteStorageBackup(ctx, snapID); err != nil {
			var svcError *upcloud.Problem
			if errors.As(err, &svcError) && svcError.Status != http.StatusNotFound {
				return nil, status.Errorf(codes.Internal, err.Error())
			}
		}
	}
	return &csi.DeleteSnapshotResponse{}, nil
}

// ListSnapshots returns the information about all snapshots on the storage
// system within the given parameters regardless of how they were created.
// ListSnapshots should not list a snapshot that is being created but has not
// been cut successfully yet.
//
// TODO OPTIONAL: implement starting token / pagination.
func (c *Controller) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	log := logger.WithServerContext(ctx, c.log).WithFields(logrus.Fields{
		logger.ListStartingTokenKey: req.GetStartingToken(),
		logger.ListMaxEntriesKey:    req.GetMaxEntries(),
		logger.VolumeSourceKey:      req.GetSourceVolumeId(),
		logger.SnapshotIDKey:        req.GetSnapshotId(),
	})

	listStart, err := parseToken(req.GetStartingToken())
	if err != nil {
		return nil, status.Error(codes.Aborted, "failed to parse starting_token")
	}

	backups := make([]upcloud.Storage, 0)

	if snapID := req.GetSnapshotId(); snapID != "" {
		log = log.WithField("snapshot_id", snapID)
		log.Info("getting storage snapshots by ID")
		s, err := c.svc.GetStorageByUUID(ctx, snapID)
		if err != nil {
			return listSnapshotsErrorResponse(err)
		}
		backups = append(backups, s.Storage)
	} else {
		log.Info("getting list of storage snapshots")
		// NOTE: SourceVolumeId can also be empty
		backups, err = c.svc.ListStorageBackups(ctx, req.GetSourceVolumeId())
		if err != nil {
			return nil, status.Errorf(codes.Internal, "listsnapshots failed with: %s", err.Error())
		}
	}
	backups, listNext := paginateStorage(backups, listStart, int(req.GetMaxEntries()))
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
	log.Infof("found %d snapshots", len(entries))
	return &csi.ListSnapshotsResponse{
		Entries:   entries,
		NextToken: fmt.Sprint(listNext),
	}, nil
}

// ControllerExpandVolume is called from the resizer to increase the volume size.
func (c *Controller) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	volumeID := req.GetVolumeId()

	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID missing in request")
	}
	log := logger.WithServerContext(ctx, c.log).WithField(logger.VolumeIDKey, req.GetVolumeId())

	log.Info("getting storage by uuid")
	volume, err := c.svc.GetStorageByUUID(ctx, volumeID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not retrieve existing volumes: %v", err)
	}

	resizeBytes, err := obtainSize(req.CapacityRange)
	if err != nil {
		return nil, status.Errorf(codes.OutOfRange, "invalid capacity range: %v", err)
	}
	resizeGigaBytes := resizeBytes / giB

	log = log.WithFields(logrus.Fields{
		"size":     volume.Size,
		"new_size": resizeGigaBytes,
	})

	if resizeGigaBytes <= int64(volume.Size) {
		log.Info("skipping volume resizeStorage because current volume size exceeds requested volume size")
		return &csi.ControllerExpandVolumeResponse{CapacityBytes: int64(volume.Size * giB), NodeExpansionRequired: true}, nil
	}

	if len(volume.ServerUUIDs) > 0 {
		return nil, status.Error(codes.FailedPrecondition, "volume is currently published on a node")
	}

	isBlockDevice := false
	if req.GetVolumeCapability() != nil {
		if _, ok := req.VolumeCapability.AccessType.(*csi.VolumeCapability_Block); ok {
			isBlockDevice = true
		}
	}

	if isBlockDevice {
		log.Info("resizing block device")
		_, err = c.svc.ResizeBlockDevice(ctx, volume.UUID, int(resizeGigaBytes))
		if err != nil {
			c.log.Errorf("cannot resizeBlockDevice volume %s: %s", volumeID, err.Error())
		}
	} else {
		log.Info("resizing volume")
		_, err = c.svc.ResizeStorage(ctx, volume.UUID, int(resizeGigaBytes), false)
		if err != nil {
			c.log.Errorf("cannot resizeStorage volume %s: %s", volumeID, err.Error())
		}
	}

	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         resizeGigaBytes * giB,
		NodeExpansionRequired: false,
	}, nil
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
		output /= tiB
		unit = "Tb"
	case inputBytes >= giB:
		output /= giB
		unit = "Gb"
	case inputBytes >= miB:
		output /= miB
		unit = "Mb"
	case inputBytes >= kiB:
		output /= kiB
		unit = "Kb"
	case inputBytes == 0:
		return "0"
	}

	result := strconv.FormatFloat(output, 'f', 1, 64)
	result = strings.TrimSuffix(result, ".0")

	return result + unit
}

func parseToken(t string) (int, error) {
	if t == "" {
		return 0, nil
	}
	return strconv.Atoi(t)
}

// paginateStorage returns slice of storages (s) and starting point of next page.
// Next page starting point is zero (0) if there isn't anymore pages left.
func paginateStorage(s []upcloud.Storage, start, size int) ([]upcloud.Storage, int) {
	var next int
	if start > len(s) {
		return s[len(s):], next
	}
	if size == 0 {
		return s[start:], next
	}
	next = (start + size)
	if next >= len(s) || size == 0 {
		s = s[start:]
		next = 0
	} else {
		s = s[start:next]
	}

	return s, next
}

func listSnapshotsErrorResponse(err error) (*csi.ListSnapshotsResponse, error) {
	if errors.Is(err, service.ErrStorageNotFound) {
		return &csi.ListSnapshotsResponse{
			Entries: make([]*csi.ListSnapshotsResponse_Entry, 0),
		}, nil
	}
	return nil, status.Error(codes.Internal, err.Error())
}

func createVolumeRequestTier(r *csi.CreateVolumeRequest) (string, error) {
	tierMapper := map[string]string{"maxiops": upcloud.StorageTierMaxIOPS, "hdd": upcloud.StorageTierHDD}
	p, ok := r.Parameters["tier"]
	if !ok {
		// tier parameter is not required
		return "", nil
	}
	tier, ok := tierMapper[p]
	if ok {
		return tier, nil
	}
	return "", status.Error(codes.InvalidArgument, fmt.Sprintf("storage tier '%s' not supported", tier))
}

func createVolumeRequestEncryptionAtRest(r *csi.CreateVolumeRequest) bool {
	e, ok := r.Parameters["encryption"]
	if ok && e == "data-at-rest" {
		return true
	}
	return false
}

func validateCreateVolumeRequest(r *csi.CreateVolumeRequest, zone string) error {
	if r.GetName() == "" {
		return status.Error(codes.InvalidArgument, "CreateVolume Name cannot be empty")
	}

	if r.GetVolumeCapabilities() == nil || len(r.VolumeCapabilities) == 0 {
		return status.Error(codes.InvalidArgument, "CreateVolume VolumeCapabilities cannot be empty")
	}

	if violations := validateCapabilities(r.VolumeCapabilities); len(violations) > 0 {
		return status.Error(codes.InvalidArgument, fmt.Sprintf("CreateVolume failed with the following violations: %s", strings.Join(violations, ", ")))
	}
	if r.GetAccessibilityRequirements() != nil {
		for _, t := range r.AccessibilityRequirements.Requisite {
			region, ok := t.Segments["region"]
			if !ok {
				continue // nothing to do
			}

			if region != zone {
				return status.Errorf(codes.ResourceExhausted, "volume can be only created in region: %q, got: %q", zone, region)
			}
		}
	}
	return nil
}

func validateControllerPublishVolumeRequest(r *csi.ControllerPublishVolumeRequest) error {
	if r.GetVolumeId() == "" {
		return status.Error(codes.InvalidArgument, "volume ID must be provided")
	}
	if r.GetNodeId() == "" {
		return status.Error(codes.InvalidArgument, "node ID must be provided")
	}
	if r.GetVolumeCapability() == nil {
		return status.Error(codes.InvalidArgument, "volume capability must be provided")
	}
	if r.GetReadonly() {
		return status.Error(codes.Unimplemented, "read only Volumes are not supported")
	}
	return nil
}
