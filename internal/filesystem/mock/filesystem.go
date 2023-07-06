package mock

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strings"

	"github.com/UpCloudLtd/upcloud-csi/internal/filesystem"
	"github.com/sirupsen/logrus"
)

type MockFilesystem struct {
	log *logrus.Logger
}

func NewFilesystem(log *logrus.Logger) filesystem.Filesystem {
	return &MockFilesystem{log: log}
}

func (m *MockFilesystem) Format(ctx context.Context, source, fsType string, mkfsArgs []string) error {
	m.log.Debugf("Mock Format(%s, %s, [%s]) -> nil", source, fsType, mkfsArgs)
	return nil
}

func (m *MockFilesystem) IsMounted(ctx context.Context, target string) (bool, error) {
	if _, err := os.Stat(target); os.IsNotExist(err) {
		m.log.Debugf("Mock IsMounted(%s) -> false, nil", target)
		return false, nil
	}
	m.log.Debugf("Mock IsMounted(%s) -> true, nil", target)
	return true, nil
}

func (m *MockFilesystem) Mount(ctx context.Context, source, target, fsType string, opts ...string) error {
	if strings.HasPrefix(target, os.TempDir()) {
		m.log.Debugf("Mock Mount(%s, %s, %s, [%s]) -> os.MkdirAll(%s) ", source, target, fsType, opts, target)
		return os.MkdirAll(target, 0o750)
	}
	m.log.Debugf("Mock Mount(%s, %s, %s, [%s]) -> nil", source, target, fsType, opts)
	return nil
}

func (m *MockFilesystem) Unmount(ctx context.Context, path string) error {
	m.log.Debugf("Mock Unmount(%s) -> nil", path)
	return nil
}

//nolint:gosec // Use of weak random number generator (math/rand instead of crypto/rand) (gosec)
func (m *MockFilesystem) Statistics(volumePath string) (filesystem.VolumeStatistics, error) {
	stats := filesystem.VolumeStatistics{
		AvailableBytes:  int64(rand.Intn(1000)),
		TotalBytes:      int64(rand.Intn(1000)),
		UsedBytes:       int64(rand.Intn(1000)),
		AvailableInodes: int64(rand.Intn(1000)),
		TotalInodes:     int64(rand.Intn(1000)),
		UsedInodes:      int64(rand.Intn(1000)),
	}
	m.log.Debugf("Mock Statistics(%s) -> %+v", volumePath, stats)
	return stats, nil
}

func (m *MockFilesystem) GetDeviceByID(ctx context.Context, id string) (string, error) {
	dev := "/dev/vda"
	m.log.Debugf("Mock GetDeviceByID(%s) -> %s", id, dev)
	return dev, nil
}

func (m *MockFilesystem) GetDeviceLastPartition(ctx context.Context, source string) (string, error) {
	if strings.HasSuffix(source, "1") {
		m.log.Debugf("Mock GetDeviceLastPartition(%s) -> %s", source, source)
		return source, nil
	}
	m.log.Debugf("Mock GetDeviceLastPartition(%s) -> %s1", source, source)
	return fmt.Sprintf("%s1", source), nil
}
