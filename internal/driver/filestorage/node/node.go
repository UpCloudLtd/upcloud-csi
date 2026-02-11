package node

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/UpCloudLtd/upcloud-csi/internal/driver/filestorage/common"
	"github.com/UpCloudLtd/upcloud-csi/internal/filesystem"
	"github.com/UpCloudLtd/upcloud-csi/internal/logger"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type node struct {
	nodeName string
	zone     string
	fs       filesystem.Filesystem
	logger   *logrus.Entry
}

func NewNode(nodeName, zone string, fs filesystem.Filesystem, logger *logrus.Entry) (csi.NodeServer, error) {
	return &node{
		nodeName: nodeName,
		zone:     zone,
		fs:       fs,
		logger:   logger,
	}, nil
}

func (n *node) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	server, ok := req.PublishContext[common.FileStorageAddressPublishContextKey]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "did not get the FileStorage address in PublishContext")
	}

	mnt := req.VolumeCapability.GetMount()
	options := mnt.GetMountFlags()

	if req.Readonly {
		options = append(options, "ro")
	}

	// while we could just as easily get this from the volumeId, this way we
	// don't have to parse it and we need the PublishContext anyway for the
	// address of the FileStorage
	shareName, ok := req.PublishContext[common.ShareNamePublishContextKey]
	if !ok {
		// TODO: should we parse the volume ID now or error out? Afaik this
		// should always be set -- Mara
		return nil, status.Errorf(codes.InvalidArgument, "%v missing from PublishContext; should have been set by controller and passed on by CO", common.ShareNamePublishContextKey)
	}

	// retrying to mount as the File Storage might not be up yet (it's only
	// started once it has a Share, ACL and Network)
	ticker := time.NewTicker(10 * time.Second)
	timeout := 6
	for {
		err := n.fs.Mount(ctx, fmt.Sprintf("%v:/%v", server, shareName), req.TargetPath, "nfs", options...)
		if err != nil {
			if err.Error() == "exit status 32" {
				<-ticker.C
				if timeout--; timeout == 0 {
					return nil, status.Error(codes.Internal, "Timeout re-trying to mount")
				}
				continue
			}

			return nil, status.Error(codes.Internal, err.Error())
		}

		break
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (n *node) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	if req.TargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "no target path given to unmount")
	}

	err := n.fs.Unmount(ctx, req.TargetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if _, err := os.Stat(req.TargetPath); err == nil {
		if err := os.Remove(req.TargetPath); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (n *node) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	log := logger.WithServerContext(ctx, n.logger)
	caps := []*csi.NodeServiceCapability{}

	log.WithField("capabilities", caps).Info("supported capabilities")
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: caps,
	}, nil
}

func (n *node) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId: n.nodeName,

		// make sure that the driver works on this particular region only
		AccessibleTopology: &csi.Topology{
			Segments: map[string]string{
				"region": n.zone,
			},
		},
	}, nil
}

func (n *node) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (n *node) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (n *node) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (n *node) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}
