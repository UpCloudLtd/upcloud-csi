package driver

import (
	"context"
	"os"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	diskIDPath = "/dev/disk/by-uuid"

	maxVolumesPerNode = 7
)

var (
	annsNoFormatVolume = []string{
		"storage.csi.upcloud.com/noformat",
	}
)

type NodeService struct {
	csi.NodeServer
}

// NodeStageVolume mounts the volume to a staging path on the node. This is
// called by the CO before NodePublishVolume and is used to temporary mount the
// volume to a staging path. Once mounted, NodePublishVolume will make sure to
// mount it to the appropriate path
func (driv *Driver) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	driv.log.Info("node stage volume called")
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
	if volName, ok := req.GetPublishContext()[driv.volumeName]; !ok {
		return nil, status.Error(codes.InvalidArgument, "Could not find the volume by name")
	} else {
		volumeName = volName
	}

	source := driv.getDiskSource(req.VolumeId)
	target := req.StagingTargetPath

	mnt := req.VolumeCapability.GetMount()
	options := mnt.MountFlags

	fsType := "ext4"
	if mnt.FsType != "" {
		fsType = mnt.FsType
	}

	nodeStageLog := driv.log.WithFields(logrus.Fields{
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
			newDevice, err := driv.mounter.isPrepared(req.VolumeId)
			if err != nil {
				return nil, err
			}
			nodeStageLog.Infof("found anonymous and unformatted device at location %s", newDevice)
			partialUUID := strings.Split(req.VolumeId, "-")[0]
			nodeStageLog.Infof("formatting %s volume for staging with partial uuid %s", newDevice, partialUUID)
			if err := driv.mounter.Format(newDevice, fsType, []string{"-L", partialUUID}); err != nil {
				nodeStageLog.Infof("error, wiping device %s", newDevice)
				driv.mounter.wipeDevice(newDevice)
				return nil, status.Error(codes.Internal, err.Error())
			}
			nodeStageLog.Infof("changing filesystem uuid to %s", req.VolumeId)
			if err := driv.mounter.setUUID(newDevice, req.VolumeId); err != nil {
				nodeStageLog.Infof("error, wiping device %s", newDevice)
				driv.mounter.wipeDevice(newDevice)
				return nil, status.Error(codes.Internal, err.Error())
			}
			nodeStageLog.Info("done preparing volume")
		} else {
			nodeStageLog.Info("expected source device location found")
			nodeStageLog.Infof("checking whether volume %s is f", source)
			f, err := driv.mounter.IsFormatted(source)
			if err != nil {
				return nil, err
			}
			if !f {
				nodeStageLog.Info("formatting the volume for staging")
				if err := driv.mounter.Format(source, fsType, []string{}); err != nil {
					return nil, status.Error(codes.Internal, err.Error())
				}
			} else {
				nodeStageLog.Info("source device is already f")
			}
		}

	}

	nodeStageLog.Info("mounting the volume for staging")

	mounted, err := driv.mounter.IsMounted(target)
	if err != nil {
		return nil, err
	}

	if !mounted {
		stageMountedLog := driv.log.WithFields(logrus.Fields{"source": source, "target": target, "fsType": fsType, "options": options})
		stageMountedLog.Info("mount options")
		if err := driv.mounter.Mount(source, target, fsType, options...); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else {
		nodeStageLog.Info("source device is already mounted to the target path")
	}
