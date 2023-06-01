package node

import (
	"context"
	"errors"
	"os"

	"github.com/UpCloudLtd/upcloud-csi/internal/filesystem"
	"github.com/UpCloudLtd/upcloud-csi/internal/logger"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	fileSystemExt4 = "ext4"
)

type Node struct {
	name string
	zone string

	maxVolumesPerNode int64

	fs  filesystem.Filesystem
	log *logrus.Entry
}

func NewNode(name, zone string, maxVolumesPerNode int64, fs filesystem.Filesystem, l *logrus.Entry) (*Node, error) {
	if name == "" {
		return nil, errors.New("node name is required field")
	}
	if zone == "" {
		return nil, errors.New("node zone is required field")
	}
	return &Node{
		name:              name,
		zone:              zone,
		maxVolumesPerNode: maxVolumesPerNode,
		fs:                fs,
		log:               l,
	}, nil
}

// NodeStageVolume mounts the volume to a staging path on the node. This is
// called by the CO before NodePublishVolume and is used to temporary mount the
// volume to a staging path. Once mounted, NodePublishVolume will make sure to
// mount it to the appropriate path.
func (n *Node) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID must be provided")
	}
	log := logger.WithServerContext(ctx, n.log).WithField(logger.VolumeIDKey, req.GetVolumeId())

	if req.StagingTargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "staging target path must be provided")
	}
	if req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "volume vapability must be provided")
	}

	target := req.GetStagingTargetPath()
	log = log.WithField(logger.MountTargetKey, target)
	// No need to stage raw block device.
	if _, ok := req.VolumeCapability.GetAccessType().(*csi.VolumeCapability_Block); ok {
		log.Info("raw block device requested")
		return &csi.NodeStageVolumeResponse{}, nil
	}

	mnt := req.VolumeCapability.GetMount()
	options := mnt.GetMountFlags()

	fsType := fileSystemExt4
	if mnt.FsType != "" {
		fsType = mnt.FsType
	}

	log.Info("getting disk source for volume ID")
	source, err := n.fs.GetDeviceByID(ctx, req.GetVolumeId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	log = log.WithFields(logrus.Fields{logger.MountSourceKey: source, "fs_type": fsType, "mount_options": options})

	log.Info("formatting the source volume for staging")
	if err := n.fs.Format(ctx, source, fsType, []string{}); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	log.Info("check if target is already mounted")
	mounted, err := n.fs.IsMounted(ctx, target)
	if err != nil {
		return nil, err
	}

	if !mounted {
		partition, err := n.fs.GetDeviceLastPartition(ctx, source)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		log.WithField("partition", partition).Info("mounting partition for staging")
		if err := n.fs.Mount(ctx, partition, target, fsType, options...); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else {
		log.Info("source device is already mounted to the target path")
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

// NodeUnstageVolume unstages the volume from the staging path and deletes the directory or file.
//
// This is a reverse operation of NodeStageVolume.
// This RPC MUST undo the work by the corresponding NodeStageVolume.
// This RPC SHALL be called by the CO once for each staging_target_path that was successfully setup via NodeStageVolume.
func (n *Node) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID must be provided")
	}
	log := logger.WithServerContext(ctx, n.log).WithField(logger.VolumeIDKey, req.GetVolumeId())

	if req.StagingTargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "staging target path must be provided")
	}

	log = log.WithField(logger.MountTargetKey, req.GetStagingTargetPath())

	log.Info("check if target is already mounted")
	mounted, err := n.fs.IsMounted(ctx, req.GetStagingTargetPath())
	if err != nil {
		return nil, err
	}

	if mounted {
		log.Info("unmounting the staging target path")
		err := n.fs.Unmount(ctx, req.GetStagingTargetPath())
		if err != nil {
			return nil, err
		}
	} else {
		log.Info("staging target path is already unmounted")
	}

	// Target path can be directory or file when access type is block
	if _, err := os.Stat(req.GetStagingTargetPath()); err == nil {
		log.Info("removing staging target path")
		if err := os.Remove(req.GetStagingTargetPath()); err != nil {
			return nil, status.Errorf(codes.Internal, err.Error())
		}
	}
	return &csi.NodeUnstageVolumeResponse{}, nil
}

// NodePublishVolume mounts the volume mounted to the staging path to the target path.
func (n *Node) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) { //nolint: funlen // TODO: refactor
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID must be provided")
	}
	log := logger.WithServerContext(ctx, n.log).WithField(logger.VolumeIDKey, req.GetVolumeId())

	if req.StagingTargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "staging target path must be provided")
	}

	if req.TargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "target path must be provided")
	}

	if req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "volume capability must be provided")
	}

	var err error
	source := req.GetStagingTargetPath()
	target := req.GetTargetPath()
	log = log.WithFields(logrus.Fields{logger.MountSourceKey: source, logger.MountTargetKey: target})

	options := []string{"bind"}
	if req.GetReadonly() {
		options = append(options, "ro")
	}
	fsType := ""
	switch req.GetVolumeCapability().GetAccessType().(type) {
	case *csi.VolumeCapability_Block:
		// raw block device requested, ignore filesystem and mount flags
		if source, err = n.fs.GetDeviceByID(ctx, req.GetVolumeId()); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

	case *csi.VolumeCapability_Mount:
		if mnt := req.VolumeCapability.GetMount(); mnt != nil {
			options = append(options, mnt.GetMountFlags()...)
			fsType = mnt.GetFsType()
		}
		if fsType == "" {
			fsType = fileSystemExt4
		}
	default:
		return nil, status.Error(codes.InvalidArgument, "unknown volume access type")
	}

	log = log.WithFields(logrus.Fields{logger.FilesystemTypeKey: fsType, logger.MountOptionsKey: options})

	log.Info("check if target is already mounted")
	mounted, err := n.fs.IsMounted(ctx, target)
	if err != nil {
		return nil, err
	}

	if !mounted {
		log.Info("mounting the volume")
		if err := n.fs.Mount(ctx, source, target, fsType, options...); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else {
		log.Info("volume is already mounted")
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume unmounts the volume from the target path and deletes the directory or file.
//
// This is a reverse operation of NodePublishVolume.
// This RPC MUST undo the work by the corresponding NodePublishVolume.
// This RPC SHALL be called by the CO at least once for each target_path that was successfully setup via NodePublishVolume.
func (n *Node) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID must be provided")
	}
	log := logger.WithServerContext(ctx, n.log).WithField(logger.VolumeIDKey, req.GetVolumeId())

	if req.GetTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "target path must be provided")
	}
	log = log.WithField(logger.MountTargetKey, req.GetTargetPath())

	log.Info("check if target is already mounted")
	mounted, err := n.fs.IsMounted(ctx, req.GetTargetPath())
	if err != nil {
		return nil, err
	}

	if mounted {
		log.Info("unmounting the target path")
		err := n.fs.Unmount(ctx, req.GetTargetPath())
		if err != nil {
			return nil, err
		}
	}
	// Target path can be directory or file when access type is block
	if _, err := os.Stat(req.GetTargetPath()); err == nil {
		log.Info("removing target path")
		if err := os.Remove(req.GetTargetPath()); err != nil {
			return nil, status.Errorf(codes.Internal, err.Error())
		}
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// NodeGetCapabilities returns the supported capabilities of the node server.
func (n *Node) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	log := logger.WithServerContext(ctx, n.log)
	caps := []*csi.NodeServiceCapability{
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

	log.WithField("capabilities", caps).Info("supported capabilities")
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: caps,
	}, nil
}

// NodeGetInfo returns the supported capabilities of the node server.
func (n *Node) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId: n.name,

		// Maximum number of volumes that controller can publish to the node.
		MaxVolumesPerNode: n.maxVolumesPerNode,

		// make sure that the driver works on this particular region only
		AccessibleTopology: &csi.Topology{
			Segments: map[string]string{
				"region": n.zone,
			},
		},
	}, nil
}

// NodeGetVolumeStats returns the volume capacity statistics available for
// the given volume.
func (n *Node) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID must be provided")
	}

	volumePath := req.GetVolumePath()
	if volumePath == "" {
		return nil, status.Error(codes.InvalidArgument, "volume path must be provided")
	}
	log := logger.WithServerContext(ctx, n.log).WithField(logger.VolumeIDKey, req.GetVolumeId()).WithField("volume_path", volumePath)

	log.Info("check if volume path is already mounted")
	mounted, err := n.fs.IsMounted(ctx, volumePath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to check if volume path %q is mounted: %s", volumePath, err)
	}

	if !mounted {
		return nil, status.Errorf(codes.NotFound, "volume path %s is not mounted", volumePath)
	}

	log.Info("getting volume path statistics")
	stats, err := n.fs.Statistics(volumePath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to retrieve capacity statistics for volume path %q: %s", volumePath, err)
	}

	log.WithField("stats", stats).Info("node capacity statistics retrieved")

	return &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Available: stats.AvailableBytes,
				Total:     stats.TotalBytes,
				Used:      stats.UsedBytes,
				Unit:      csi.VolumeUsage_BYTES,
			},
			{
				Available: stats.AvailableInodes,
				Total:     stats.TotalInodes,
				Used:      stats.UsedInodes,
				Unit:      csi.VolumeUsage_INODES,
			},
		},
	}, nil
}

func (n *Node) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method NodeExpandVolume not implemented")
}
