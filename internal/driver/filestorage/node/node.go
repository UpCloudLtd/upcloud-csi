package node

import (
	"context"

	"github.com/UpCloudLtd/upcloud-csi/internal/filesystem"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"
)

type node struct {
	nodeName string
	fs       filesystem.Filesystem
	logger   *logrus.Entry
}

func NewNode(nodeName string, fs filesystem.Filesystem, logger *logrus.Entry) (csi.NodeServer, error) {
	return &node{
		nodeName: nodeName,
		fs:       fs,
		logger:   logger,
	}, nil
}

func (n *node) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (n *node) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (n *node) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (n *node) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (n *node) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (n *node) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (n *node) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	panic("not implemented") // TODO: Implement
}

func (n *node) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	panic("not implemented") // TODO: Implement
}
