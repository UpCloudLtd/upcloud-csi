package driver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	udevDiskByIDPath  = "/dev/disk/by-id"
	diskPrefix        = "virtio-"
	maxVolumesPerNode = 7
)

var errNodeDiskNotFound = errors.New("disk not found")

// NodeStageVolume mounts the volume to a staging path on the node. This is
// called by the CO before NodePublishVolume and is used to temporary mount the
// volume to a staging path. Once mounted, NodePublishVolume will make sure to
// mount it to the appropriate path.
func (d *Driver) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID must be provided")
	}
	log := logWithServerContext(d.log, ctx).WithField(logVolumeIDKey, req.GetVolumeId())

	if req.StagingTargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "staging target path must be provided")
	}
	if req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "volume vapability must be provided")
	}

	target := req.GetStagingTargetPath()
	log = log.WithField(logMountTargetKey, target)
	// No need to stage raw block device.
	switch req.VolumeCapability.GetAccessType().(type) {
	case *csi.VolumeCapability_Block:
		log.Info("raw block device requested")
		return &csi.NodeStageVolumeResponse{}, nil
	}

	mnt := req.VolumeCapability.GetMount()
	options := mnt.GetMountFlags()

	fsType := "ext4"
	if mnt.FsType != "" {
		fsType = mnt.FsType
	}

	log.Info("getting disk source for volume ID")
	source, err := getBlockDeviceByVolumeID(ctx, req.GetVolumeId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	log = log.WithFields(logrus.Fields{logMountSourceKey: source, "fs_type": fsType, "mount_options": options})

	log.Info("checking whether source if formatted")
	formatted, err := d.mounter.IsFormatted(ctx, source)
	if err != nil {
		return nil, err
	}
	if !formatted {
		log.Info("formatting the source volume for staging")
		if err := d.mounter.Format(ctx, source, fsType, []string{}); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else {
		log.Info("source device is already formatted")
	}

	log.Info("check if target is already mounted")
	mounted, err := d.mounter.IsMounted(ctx, target)
	if err != nil {
		return nil, err
	}

	if !mounted {
		partition, err := getLastPartition(ctx, source)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		log.WithField("partition", partition).Info("mounting partition for staging")
		if err := d.mounter.Mount(ctx, partition, target, fsType, options...); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else {
		log.Info("source device is already mounted to the target path")
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

// NodeUnstageVolume unstages the volume from the staging path.
func (d *Driver) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID must be provided")
	}
	log := logWithServerContext(d.log, ctx).WithField(logVolumeIDKey, req.GetVolumeId())

	if req.StagingTargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "staging target path must be provided")
	}

	log = log.WithField(logMountTargetKey, req.GetStagingTargetPath())

	log.Info("check if target is already mounted")
	mounted, err := d.mounter.IsMounted(ctx, req.StagingTargetPath)
	if err != nil {
		return nil, err
	}

	if mounted {
		log.Info("unmounting the staging target path")
		err := d.mounter.Unmount(ctx, req.StagingTargetPath)
		if err != nil {
			return nil, err
		}
	} else {
		log.Info("staging target path is already unmounted")
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

// NodePublishVolume mounts the volume mounted to the staging path to the target path.
func (d *Driver) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID must be provided")
	}
	log := logWithServerContext(d.log, ctx).WithField(logVolumeIDKey, req.GetVolumeId())

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
	log = log.WithFields(logrus.Fields{logMountSourceKey: source, logMountTargetKey: target})

	options := []string{"bind"}
	if req.GetReadonly() {
		options = append(options, "ro")
	}
	fsType := ""
	switch req.GetVolumeCapability().GetAccessType().(type) {
	case *csi.VolumeCapability_Block:
		// raw block device requested, ignore filesystem and mount flags
		if source, err = getBlockDeviceByVolumeID(ctx, req.GetVolumeId()); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

	case *csi.VolumeCapability_Mount:
		if mnt := req.VolumeCapability.GetMount(); mnt != nil {
			options = append(options, mnt.GetMountFlags()...)
			fsType = mnt.GetFsType()
		}
		if fsType == "" {
			fsType = "ext4"
		}
	default:
		return nil, status.Error(codes.InvalidArgument, "unknown volume access type")
	}

	log = log.WithFields(logrus.Fields{logFilesystemTypeKey: fsType, logMountOptionsKey: options})

	log.Info("check if target is already mounted")
	mounted, err := d.mounter.IsMounted(ctx, target)
	if err != nil {
		return nil, err
	}

	if !mounted {
		log.Info("mounting the volume")
		if err := d.mounter.Mount(ctx, source, target, fsType, options...); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else {
		log.Info("volume is already mounted")
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume unmounts the volume from the target path and deletes the directory.
func (d *Driver) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID must be provided")
	}
	log := logWithServerContext(d.log, ctx).WithField(logVolumeIDKey, req.GetVolumeId())

	if req.TargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "target path must be provided")
	}
	log = log.WithField(logMountTargetKey, req.GetTargetPath())

	log.Info("check if target is already mounted")
	mounted, err := d.mounter.IsMounted(ctx, req.TargetPath)
	if err != nil {
		return nil, err
	}

	if mounted {
		log.Info("unmounting the target path")
		err := d.mounter.Unmount(ctx, req.TargetPath)
		if err != nil {
			return nil, err
		}
	}
	targetInfo, err := os.Stat(req.GetTargetPath())
	if err == nil && targetInfo.IsDir() {
		log.Info("removing target path")
		if err := os.Remove(req.GetTargetPath()); err != nil {
			return nil, status.Errorf(codes.Internal, err.Error())
		}
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// NodeGetCapabilities returns the supported capabilities of the node server.
func (d *Driver) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	log := logWithServerContext(d.log, ctx)
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
func (d *Driver) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
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
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID must be provided")
	}

	volumePath := req.GetVolumePath()
	if volumePath == "" {
		return nil, status.Error(codes.InvalidArgument, "volume path must be provided")
	}
	log := logWithServerContext(d.log, ctx).WithField(logVolumeIDKey, req.GetVolumeId()).WithField("volume_path", volumePath)

	log.Info("check if volume path is already mounted")
	mounted, err := d.mounter.IsMounted(ctx, volumePath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to check if volume path %q is mounted: %s", volumePath, err)
	}

	if !mounted {
		return nil, status.Errorf(codes.NotFound, "volume path %s is not mounted", volumePath)
	}

	log.Info("getting volume path statistics")
	stats, err := d.mounter.GetStatistics(ctx, volumePath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to retrieve capacity statistics for volume path %q: %s", volumePath, err)
	}

	log.WithField("stats", stats).Info("node capacity statistics retrieved")

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

// getBlockDeviceByVolumeID returns the absolute path of the attached block device for the given volumeID.
func getBlockDeviceByVolumeID(ctx context.Context, volumeID string) (string, error) {
	diskID, err := volumeIDToDiskID(volumeID)
	if err != nil {
		return diskID, err
	}
	return getBlockDeviceByDiskID(ctx, diskID)
}

// getBlockDeviceByDiskID returns actual block device path (e.g. /dev/vda) that correspond to disk ID (hardware serial number).
// diskID can be udev disk ID or path to disk ID symbolic link e.g. /dev/disk/by-id/virtio-014e425736724563ab83
func getBlockDeviceByDiskID(ctx context.Context, diskID string) (dev string, err error) {
	ln := diskID
	if !filepath.IsAbs(diskID) {
		ln = filepath.Join(udevDiskByIDPath, diskID)
	}

	if err := udevWaitDiskToSettle(ctx, ln); err != nil {
		return ln, err
	}

	for s := time.Now(); time.Since(s) < time.Second*udevDiskTimeout; {
		dev, err = os.Readlink(ln)
		if err != nil && !os.IsNotExist(err) {
			return ln, err
		}
		if dev != "" {
			break
		}
		time.Sleep(time.Second * 2)
	}

	if dev == "" {
		return ln, errNodeDiskNotFound
	}

	if !filepath.IsAbs(dev) {
		dev, err = filepath.Abs(filepath.Join(filepath.Dir(ln), dev))
		if err != nil {
			return dev, err
		}
	}
	_, err = os.Stat(dev)
	return dev, err
}

// udevWaitDiskToSettle uses udevadm to wait events in event queue to be handled.
func udevWaitDiskToSettle(ctx context.Context, path string) error {
	return exec.CommandContext(ctx,
		"udevadm",
		"settle",
		fmt.Sprintf("--timeout=%d", udevSettleTimeout*time.Second),
		fmt.Sprintf("--exit-if-exists=%s", path),
	).Run()
}

// volumeIDToDiskID converts volume ID to disk ID managed by udev e.g. f67db1ca-825b-40aa-a6f4-390ac6ff1b91 -> virtio-f67db1ca825b40aaa6f4
func volumeIDToDiskID(volumeID string) (string, error) {
	fullId := strings.Join(strings.Split(volumeID, "-"), "")
	if len(fullId) <= 20 {
		return "", fmt.Errorf("volume ID '%s' too short", volumeID)
	}
	return diskPrefix + fullId[:20], nil
}
