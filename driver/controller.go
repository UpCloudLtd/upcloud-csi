package driver

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/UpCloudLtd/upcloud-go-api/v4/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v4/upcloud/client"
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

// CreateVolume provisions storage via UpCloud Storage service.
func (d *Driver) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (resp *csi.CreateVolumeResponse, err error) {
	log := logWithServerContext(d.log, ctx).WithField(logVolumeNameKey, req.GetName())

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

		log.WithField(logVolumeIDKey, vol.UUID).Info("volume already exists")
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
		log := log.WithField(logVolumeSourceKey, sourceID)
		log.Info("getting source storage by uuid")
		src, err := d.upclouddriver.getStorageByUUID(ctx, sourceID)
		if err != nil {
			if err == errUpCloudStorageNotFound {
				return nil, status.Errorf(codes.NotFound, "could not retrieve source volume by ID: %v", err)
			}
			return nil, status.Errorf(codes.InvalidArgument, err.Error())
		}
		log.Info("checking that source storage is online")
		if err := d.upclouddriver.requireStorageOnline(ctx, &src.Storage); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		volumeReq := &request.CloneStorageRequest{
			UUID:  src.Storage.UUID,
			Zone:  d.options.zone,
			Tier:  tier,
			Title: volumeName,
		}
		logWithServiceRequest(log, volumeReq).Info("cloning volume")
		vol, err = d.upclouddriver.cloneStorage(ctx, volumeReq)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		log = log.WithField(logVolumeIDKey, vol.Storage.UUID).WithField("size", vol.Storage.Size)
		if storageSizeGB > vol.Storage.Size {
			log.WithField("new_size", storageSizeGB).Info("resizing volume")
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

		logWithServiceRequest(log, volumeReq).Info("creating volume")
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

	return volResp, nil
}

// DeleteVolume deletes storage via UpCloud Storage service.
func (d *Driver) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "DeleteVolume Volume ID must be provided")
	}

	logWithServerContext(d.log, ctx).WithField(logVolumeIDKey, req.GetVolumeId()).Info("deleting volume")
	err := d.upclouddriver.deleteStorage(ctx, req.VolumeId)
	if err != nil && err != errUpCloudStorageNotFound {
		return &csi.DeleteVolumeResponse{}, err
	}
	return &csi.DeleteVolumeResponse{}, nil

}

// ControllerPublishVolume attaches storage to a node via UpCloud Storage service.
func (d *Driver) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID must be provided")
	}
	log := logWithServerContext(d.log, ctx).WithField(logVolumeIDKey, req.GetVolumeId())

	if req.NodeId == "" {
		return nil, status.Error(codes.InvalidArgument, "node ID must be provided")
	}
	log = log.WithField(logNodeIDKey, req.GetNodeId())

	if req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "volume capability must be provided")
	}
	if req.GetReadonly() {
		return nil, status.Error(codes.Unimplemented, "read only Volumes are not supported")
	}

	server, err := d.upclouddriver.getServerByHostname(ctx, req.NodeId)
	if err != nil {
		if err == errUpCloudServerNotFound {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// check if volume exist before trying to attach it
	log.Info("getting storage by uuid")
	volume, err := d.upclouddriver.getStorageByUUID(ctx, req.VolumeId)
	if err != nil {
		if err == errUpCloudStorageNotFound {
			return nil, status.Error(codes.NotFound, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	log.Info("checking that storage is online")
	if err = d.upclouddriver.requireStorageOnline(ctx, &volume.Storage); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	attachedID := ""
	for _, id := range volume.ServerUUIDs {
		attachedID = id
		if id == server.UUID {
			log.Info("volume is already attached")
			return &csi.ControllerPublishVolumeResponse{
				PublishContext: map[string]string{
					d.options.volumeName:        volume.Title,
					string(ctxCorrelationIDKey): contextCorrelationID(ctx),
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

	log.Info("attaching storage to node")
	err = d.upclouddriver.attachStorage(ctx, req.VolumeId, server.UUID)
	if err != nil {
		// already attached to the node
		return nil, err
	}

	return &csi.ControllerPublishVolumeResponse{
		PublishContext: map[string]string{
			d.options.volumeName:        volume.Title,
			string(ctxCorrelationIDKey): contextCorrelationID(ctx),
		},
	}, nil
}

// ControllerUnpublishVolume detaches storage from a node via UpCloud Storage service.
func (d *Driver) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID must be provided")
	}
	log := logWithServerContext(d.log, ctx).WithField(logVolumeIDKey, req.GetVolumeId())

	log.Info("getting storage by uuid")
	// check if volume exist before trying to detach it
	_, err := d.upclouddriver.getStorageByUUID(ctx, req.GetVolumeId())
	if err != nil {
		if err == errUpCloudStorageNotFound {
			return &csi.ControllerUnpublishVolumeResponse{}, nil
		}
		return nil, err
	}

	// TODO:  If node ID is not set, the SP MUST unpublish the volume from all nodes it is published to.
	log.Info("getting server by hostname")
	server, err := d.upclouddriver.getServerByHostname(ctx, req.GetNodeId())
	if err != nil {
		return nil, err
	}

	log.Info("detaching volume")
	err = d.upclouddriver.detachStorage(ctx, req.VolumeId, server.UUID)
	if err != nil {
		return nil, err
	}

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (d *Driver) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	// ALPHA FEATURE
	// This optional RPC MAY be called by the CO to fetch current information about a volume.
	// A Controller Plugin MUST implement this ControllerGetVolume RPC call if it has GET_VOLUME capability.
	// When implemented add csi.ControllerServiceCapability_RPC_GET_VOLUME to supportedCapabilities.
	return nil, status.Errorf(codes.Unimplemented, "method ControllerGetVolume not implemented")
}

// ValidateVolumeCapabilities checks if the volume capabilities are valid.
func (d *Driver) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID must be provided")
	}
	log := logWithServerContext(d.log, ctx).WithField(logVolumeIDKey, req.GetVolumeId())

	if req.VolumeCapabilities == nil {
		return nil, status.Error(codes.InvalidArgument, "volume vapabilities must be provided")
	}

	log.Info("getting storage by uuid")
	// check if volume exist before trying to validate it
	if _, err := d.upclouddriver.getStorageByUUID(ctx, req.VolumeId); err != nil {
		if err == errUpCloudStorageNotFound {
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
// TODO OPTIONAL: implement starting token / pagination
func (d *Driver) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	log := logWithServerContext(d.log, ctx).WithFields(logrus.Fields{
		logListStartingTokenKey: req.GetStartingToken(),
		logListMaxEntriesKey:    req.GetMaxEntries(),
	})
	listStart, err := parseToken(req.GetStartingToken())
	if err != nil {
		return nil, status.Error(codes.Aborted, "failed to parse starting_token")
	}
	log.Info("getting list of storages")
	volumes, err := d.upclouddriver.listStorage(ctx, d.options.zone)
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
func (d *Driver) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// ControllerGetCapabilities returns the capacity of the storage pool.
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

	logWithServerContext(d.log, ctx).WithField("caps", caps).Info("reporting capabilities")
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: caps,
	}, nil
}

// CreateSnapshot will be called by the CO to create a new snapshot from a
// source volume on behalf of a user.
func (d *Driver) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "snapshot name must be provided")
	}
	if req.GetSourceVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "snapshot source volume ID must be provided")
	}
	log := logWithServerContext(d.log, ctx)
	log.Info("getting storage backup by name")
	s, err := d.upclouddriver.getStorageBackupByName(ctx, req.GetName())
	if err != nil && err != errUpCloudStorageNotFound {
		return nil, status.Errorf(codes.Internal, "createsnapshot failed with: %s", err.Error())
	}

	if s != nil && s.Origin != req.GetSourceVolumeId() {
		return nil, status.Error(codes.AlreadyExists, "snapshot already exists with different source volume ID")
	}
	if s == nil {
		log.Info("creating strorage backup")
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
	if req.GetSnapshotId() == "" {
		return nil, status.Error(codes.InvalidArgument, "snapshot ID must be provided")
	}
	if err := d.upclouddriver.deleteStorageBackup(ctx, req.GetSnapshotId()); err != nil {
		if e, ok := err.(*client.Error); ok && e.ErrorCode != http.StatusNotFound {
			return nil, status.Errorf(codes.Internal, err.Error())
		}
	}
	return &csi.DeleteSnapshotResponse{}, nil
}

// ListSnapshots returns the information about all snapshots on the storage
// system within the given parameters regardless of how they were created.
// ListSnapshots should not list a snapshot that is being created but has not
// been cut successfully yet.
//
// TODO OPTIONAL: implement starting token / pagination
func (d *Driver) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	log := logWithServerContext(d.log, ctx).WithFields(logrus.Fields{
		logListStartingTokenKey: req.GetStartingToken(),
		logListMaxEntriesKey:    req.GetMaxEntries(),
		logVolumeSourceKey:      req.GetSourceVolumeId(),
		logSnapshotIDKey:        req.GetSnapshotId(),
	})

	listStart, err := parseToken(req.GetStartingToken())
	if err != nil {
		return nil, status.Error(codes.Aborted, "failed to parse starting_token")
	}

	backups := make([]upcloud.Storage, 0)

	if snapID := req.GetSnapshotId(); snapID != "" {
		log = log.WithField("snapshot_id", snapID)
		log.Info("getting storage snapshots by ID")
		s, err := d.upclouddriver.getStorageByUUID(ctx, snapID)
		if err != nil {
			if err == errUpCloudStorageNotFound {
				return &csi.ListSnapshotsResponse{
					Entries: make([]*csi.ListSnapshotsResponse_Entry, 0),
				}, nil
			}
			return nil, status.Error(codes.Internal, err.Error())
		}
		backups = append(backups, s.Storage)
	} else {
		log.Info("getting list of storage snapshots")
		// NOTE: SourceVolumeId can also be empty
		backups, err = d.upclouddriver.listStorageBackups(ctx, req.GetSourceVolumeId())
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
func (d *Driver) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	volumeId := req.GetVolumeId()

	if volumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID missing in request")
	}
	log := logWithServerContext(d.log, ctx).WithField(logVolumeIDKey, req.GetVolumeId())

	log.Info("getting storage by uuid")
	volume, err := d.upclouddriver.getStorageByUUID(ctx, volumeId)
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
		_, err = d.upclouddriver.resizeBlockDevice(ctx, volume.UUID, int(resizeGigaBytes))
		if err != nil {
			d.log.Errorf("cannot resizeBlockDevice volume %s: %s", volumeId, err.Error())
		}
	} else {
		log.Info("resizing volume")
		_, err = d.upclouddriver.resizeStorage(ctx, volume.UUID, int(resizeGigaBytes), false)
		if err != nil {
			d.log.Errorf("cannot resizeStorage volume %s: %s", volumeId, err.Error())
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
