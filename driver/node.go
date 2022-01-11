package driver

import (
	"context"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxVolumesPerNode = 7
	diskIDPath        = "/dev/disk/uuid"
)

type NodeService struct {
	Driver *Driver
	csi.NodeServer
}

func (node *NodeService) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	if len(req.VolumeId) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume Volume ID must be provided")
	}

	if len(req.StagingTargetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume Staging Target Path must be provided")
	}

	if req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume Volume Capability must be provided")
	}

	volumeName := ""
	if volName, ok := req.GetPublishContext()[node.Driver.volumeName]; !ok {
		return nil, status.Error(codes.InvalidArgument, "Could not find the volume by name")
	} else {
		volumeName = volName
	}

	source := filepath.Join(diskIDPath, req.VolumeId)
	target := req.StagingTargetPath

	mnt := req.VolumeCapability.GetMount()
	options := mnt.MountFlags

	fsType := "ext4"
	if mnt.FsType != "" {
		fsType = mnt.FsType
	}

	log := node.Driver.log.With(logrus.Fields{
		"volume_id":           req.VolumeId,
		"volume_name":         volumeName,
		"volume_context":      req.VolumeContext,
		"publish_context":     req.PublishContext,
		"staging_target_path": req.StagingTargetPath,
		"source":              source,
		"fsType":              fsType,
		"mount_options":       options,
		"method":              "node_stage_volume",
	})

	log.Info("expected source device location: %s", source)
	_, err := os.Stat(source)
	if os.IsNotExist(err) {
		log.Info("expected source device location not found. checking whether device present and identifiable")

		err := node.Driver.mounter.FormatAndMount(source, target, fsType, options)
		if err != nil {
			log.Error("failed to format and mount the device")
			return nil, err
		}

		log.Info("found anonymous and unformatted device at location %s", newDevice)
		partialUUID := strings.Split(req.VolumeId, "-")[0]
		log.Info("formatting %s volume for staging with partial uuid %s", newDevice, partialUUID)
		if err := node.Driver.mounter.Format(newDevice, fsType, []string{"-L", partialUUID}); err != nil {
			log.Info("error, wiping device %s", newDevice)
			node.Driver.mounter.wipeDevice(newDevice)
			return nil, status.Error(codes.Internal, err.Error())
		}
		log.Info("changing filesystem uuid to %s", req.VolumeId)
		if err := node.Driver.mounter.setUUID(newDevice, req.VolumeId); err != nil {
			log.Info("error, wiping device %s", newDevice)
			node.Driver.mounter.wipeDevice(newDevice)
			return nil, status.Error(codes.Internal, err.Error())
		}
		log.Info("done preparing volume")
	} else {
		log.Info("expected source device location found")
		log.Info("checking whether volume %s is formatted", source)
		formatted, err := node.Driver.mounter.IsFormatted(source)
		if err != nil {
			return nil, err
		}
		if !formatted {
			log.Info("formatting the volume for staging")
			if err := node.Driver.mounter.Format(source, fsType, []string{}); err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		} else {
			log.Info("source device is already formatted")
		}
	}
}

log.Info("mounting the volume for staging")

mounted, err := node.Driver.mounter.IsMounted(target)
if err != nil {
return nil, err
}

if !mounted {
mountedLog := node.Driver.log.With("source", source).With("target", target).With(
"fsType", fsType).With("options", options)
mountedLog.Info("mount options")
if err := node.Driver.mounter.Mount(source, target, fsType, options...); err != nil {
return nil, status.Error(codes.Internal, err.Error())
}
} else {
log.Info("source device is already mounted to the target path")
}

log.Info("formatting and mounting stage volume is finished")
return &csi.NodeStageVolumeResponse{}, nil

}
