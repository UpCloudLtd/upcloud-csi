package driver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	diskIDPath        = "/dev/disk/by-id"
	diskPrefix        = "virtio-"
	maxVolumesPerNode = 7
)

var annsNoFormatVolume = []string{
	"storage.csi.upcloud.com/noformat",
}

// NodeStageVolume mounts the volume to a staging path on the node. This is
// called by the CO before NodePublishVolume and is used to temporary mount the
// volume to a staging path. Once mounted, NodePublishVolume will make sure to
// mount it to the appropriate path.
func (d *Driver) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	d.log.Info("node stage volume called")
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume Volume ID must be provided")
	}

	if req.StagingTargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume Staging Target Path must be provided")
	}

	if req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume Volume Capability must be provided")
	}

	volumeName := ""
	if volName, ok := req.GetPublishContext()[d.options.volumeName]; !ok {
		return nil, status.Error(codes.InvalidArgument, "Could not find the volume by driverName")
	} else {
		volumeName = volName
	}

	source := d.getDiskSource(req.VolumeId)
	target := req.StagingTargetPath

	mnt := req.VolumeCapability.GetMount()
	options := mnt.MountFlags

	fsType := "ext4"
	if mnt.FsType != "" {
		fsType = mnt.FsType
	}

	nodeStageLog := d.log.WithFields(logrus.Fields{
		"volume_id":           req.VolumeId,
		"volume_name":         volumeName,
		"volume_context":      req.VolumeContext,
		"publish_context":     req.PublishContext,
		"staging_target_path": target,
		"source":              source,
		"fsType":              fsType,
		"mount_options":       options,
		"method":              "node_stage_volume",
	})

	var noFormat bool
	for _, ann := range annsNoFormatVolume {
		_, noFormat = req.VolumeContext[ann]
		if noFormat {
			break
		}
	}
	if noFormat {
		nodeStageLog.Info("skipping formatting the source device")
	} else {
		nodeStageLog.Infof("expected source device location: %s", source)
		_, err := os.Stat(source)
		if os.IsNotExist(err) {
			nodeStageLog.Info("expected source device location not found. checking whether device present and identifiable")
			newDevice, err := d.mounter.isPrepared(source)
			if err != nil {
				return nil, err
			}
			nodeStageLog.Infof("found anonymous and unformatted device at location %s", newDevice)
			partialUUID := strings.Split(req.VolumeId, "-")[0]
			nodeStageLog.Infof("formatting %s volume for staging with partial uuid %s", newDevice, partialUUID)
			if err := d.mounter.Format(newDevice, fsType, []string{"-L", partialUUID}); err != nil {
				nodeStageLog.Infof("error, wiping device %s", newDevice)
				if err := d.mounter.wipeDevice(newDevice); err != nil {
					d.log.WithFields(logrus.Fields{"device": newDevice}).Infof("wiping device failed: %s", err.Error())
				}
				return nil, status.Error(codes.Internal, err.Error())
			}
			nodeStageLog.Infof("changing filesystem uuid to %s", req.VolumeId)
			if err := d.mounter.setUUID(newDevice, req.VolumeId); err != nil {
				nodeStageLog.Infof("error, wiping device %s", newDevice)
				if err := d.mounter.wipeDevice(newDevice); err != nil {
					d.log.WithFields(logrus.Fields{"device": newDevice}).Infof("wiping device failed: %s", err.Error())
				}
				return nil, status.Error(codes.Internal, err.Error())
			}
			nodeStageLog.Info("done preparing volume")
		} else {
			nodeStageLog.Info("expected source device location found")
			nodeStageLog.Infof("checking whether volume %s is formatted", source)
			formatted, err := d.mounter.IsFormatted(source)
			if err != nil {
				return nil, err
			}
			if !formatted {
				nodeStageLog.Info("formatting the volume for staging")
				if err := d.mounter.Format(source, fsType, []string{}); err != nil {
					return nil, status.Error(codes.Internal, err.Error())
				}
			} else {
				nodeStageLog.Info("source device is already formatted")
			}
		}
	}

	nodeStageLog.Info("mounting the volume for staging")

	mounted, err := d.mounter.IsMounted(target)
	if err != nil {
		return nil, err
	}

	if !mounted {
		partition, err := getLastPartition(source)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		stageMountedLog := d.log.WithFields(logrus.Fields{
			"source":    source,
			"partition": partition,
			"target":    target,
			"fsType":    fsType,
			"options":   options,
		})
		stageMountedLog.Info("mount options")
		if err := d.mounter.Mount(partition, target, fsType, options...); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else {
		nodeStageLog.Info("source device is already mounted to the target path")
	}

	nodeStageLog.Info("formatting and mounting stage volume is finished")
	return &csi.NodeStageVolumeResponse{}, nil
}

// NodeUnstageVolume unstages the volume from the staging path.
func (d *Driver) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnstageVolume Volume ID must be provided")
	}

	if req.StagingTargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnstageVolume Staging Target Path must be provided")
	}

	nodeUnstageLog := d.log.WithFields(logrus.Fields{
		"volume_id":           req.VolumeId,
		"staging_target_path": req.StagingTargetPath,
		"method":              "node_unstage_volume",
	})
	nodeUnstageLog.Info("node unstage volume called")

	mounted, err := d.mounter.IsMounted(req.StagingTargetPath)
	if err != nil {
		return nil, err
	}

	if mounted {
		nodeUnstageLog.Info("unmounting the staging target path")
		err := d.mounter.Unmount(req.StagingTargetPath)
		if err != nil {
			return nil, err
		}
	} else {
		nodeUnstageLog.Info("staging target path is already unmounted")
	}

	nodeUnstageLog.Info("unmounting stage volume is finished")
	return &csi.NodeUnstageVolumeResponse{}, nil
}

// NodePublishVolume mounts the volume mounted to the staging path to the target path.
func (d *Driver) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	d.log.Info("node publish volume called")
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Volume ID must be provided")
	}

	if req.StagingTargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Staging Target Path must be provided")
	}

	if req.TargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Target Path must be provided")
	}

	if req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "NodePublishVolume Volume Capability must be provided")
	}

	source := req.StagingTargetPath
	target := req.TargetPath

	mnt := req.VolumeCapability.GetMount()
	options := mnt.MountFlags

	options = append(options, "bind")
	if req.Readonly {
		options = append(options, "ro")
	}

	fsType := "ext4"
	if mnt.FsType != "" {
		fsType = mnt.FsType
	}

	nodePublishVolumeLog := d.log.WithFields(logrus.Fields{
		"volume_id":     req.VolumeId,
		"source":        source,
		"target":        target,
		"fsType":        fsType,
		"mount_options": options,
		"method":        "node_publish_volume",
	})

	mounted, err := d.mounter.IsMounted(target)
	if err != nil {
		return nil, err
	}

	if !mounted {
		nodePublishVolumeLog.Info("mounting the volume")
		if err := d.mounter.Mount(source, target, fsType, options...); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else {
		nodePublishVolumeLog.Info("volume is already mounted")
	}

	nodePublishVolumeLog.Info("bind mounting the volume is finished")
	return &csi.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume unmounts the volume from the target path.
func (d *Driver) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume Volume ID must be provided")
	}

	if req.TargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeUnpublishVolume Target Path must be provided")
	}

	nodeUnpublishVolumeLog := d.log.WithFields(logrus.Fields{
		"volume_id":   req.VolumeId,
		"target_path": req.TargetPath,
		"method":      "node_unpublish_volume",
	})
	nodeUnpublishVolumeLog.Info("node unpublish volume called")

	mounted, err := d.mounter.IsMounted(req.TargetPath)
	if err != nil {
		return nil, err
	}

	if mounted {
		nodeUnpublishVolumeLog.Info("unmounting the target path")
		err := d.mounter.Unmount(req.TargetPath)
		if err != nil {
			return nil, err
		}
	} else {
		nodeUnpublishVolumeLog.Info("target path is already unmounted")
	}

	nodeUnpublishVolumeLog.Info("unmounting volume is finished")
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// NodeGetCapabilities returns the supported capabilities of the node server.
func (d *Driver) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	nscaps := []*csi.NodeServiceCapability{
		{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
				},
			},
		},
		{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
				},
			},
		},
	}

	d.log.WithFields(logrus.Fields{
		"node_capabilities": nscaps,
		"method":            "node_get_capabilities",
	}).Info("node get capabilities called")
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: nscaps,
	}, nil
}

// NodeGetInfo returns the supported capabilities of the node server.
func (d *Driver) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	d.log.WithField("method", "node_get_info").Info("node get info called")
	return &csi.NodeGetInfoResponse{
		NodeId:            d.options.nodeHost,
		MaxVolumesPerNode: maxVolumesPerNode,

		// make sure that the driver works on this particular region only
		AccessibleTopology: &csi.Topology{
			Segments: map[string]string{
				"region": d.options.zone,
			},
		},
	}, nil
}

// NodeGetVolumeStats returns the volume capacity statistics available for
// the given volume.
func (d *Driver) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	nodeGetVolumeStatsLog := d.log.WithField("method", "node_get_volume_stats")
	nodeGetVolumeStatsLog.Info("node get volume stats called")

	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeGetVolumeStats Volume ID must be provided")
	}

	volumePath := req.VolumePath
	if volumePath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeGetVolumeStats Volume Path must be provided")
	}

	mounted, err := d.mounter.IsMounted(volumePath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to check if volume path %q is mounted: %s", volumePath, err)
	}

	if !mounted {
		return nil, status.Errorf(codes.NotFound, "volume path %q is not mounted", volumePath)
	}

	stats, err := d.mounter.GetStatistics(volumePath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to retrieve capacity statistics for volume path %q: %s", volumePath, err)
	}

	nodeGetVolumeStatsLog.WithFields(logrus.Fields{
		"bytes_available":  stats.availableBytes,
		"bytes_total":      stats.totalBytes,
		"bytes_used":       stats.usedBytes,
		"inodes_available": stats.availableInodes,
		"inodes_total":     stats.totalInodes,
		"inodes_used":      stats.usedInodes,
	}).Info("node capacity statistics retrieved")

	return &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Available: stats.availableBytes,
				Total:     stats.totalBytes,
				Used:      stats.usedBytes,
				Unit:      csi.VolumeUsage_BYTES,
			},
			{
				Available: stats.availableInodes,
				Total:     stats.totalInodes,
				Used:      stats.usedInodes,
				Unit:      csi.VolumeUsage_INODES,
			},
		},
	}, nil
}

func (d *Driver) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method NodeExpandVolume not implemented")
}

// getDiskSource returns the absolute path of the attached volume for the given volumeID.
func (d *Driver) getDiskSource(volumeID string) string {
	diskID := volumeIDToDiskID(volumeID)
	if diskID == "" {
		return ""
	}
	return getDiskByID(diskID, "")
}

func getDiskByID(diskID, basePath string) string {
	if basePath == "" {
		basePath = diskIDPath
	}
	link, err := os.Readlink(filepath.Join(basePath, diskPrefix+diskID))
	if err != nil {
		fmt.Println(fmt.Errorf("failed to get the link to source"))
		return ""
	}
	if filepath.IsAbs(link) {
		return link
	}

	return filepath.Join(basePath, link)
}

func volumeIDToDiskID(volumeID string) string {
	fullId := strings.Join(strings.Split(volumeID, "-"), "")
	if len(fullId) <= 20 {
		return ""
	}
	return fullId[:20]
}
