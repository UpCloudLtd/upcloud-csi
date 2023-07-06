package node_test

import (
	"context"
	"testing"

	"github.com/UpCloudLtd/upcloud-csi/internal/filesystem/mock"
	"github.com/UpCloudLtd/upcloud-csi/internal/node"
	"github.com/sirupsen/logrus"
)

func TestNode_ExpandVolume(t *testing.T) {
	t.Parallel()
	logger := logrus.New()
	d, _ := node.NewNode("test-node", "fi-hel1", 10, mock.NewFilesystem(logger), logger.WithField("package", "node_test"))
	if _, err := d.NodeExpandVolume(context.TODO(), nil); err == nil {
		t.Error("NodeExpandVolume should return error. Only offline volume expansion is supported and it's handled by controller.")
	}
}
