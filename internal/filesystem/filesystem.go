package filesystem

import (
	"context"
)

type VolumeStatistics struct {
	AvailableBytes,
	TotalBytes,
	UsedBytes,
	AvailableInodes,
	TotalInodes,
	UsedInodes int64
}

type Filesystem interface {
	Format(ctx context.Context, source, fsType string, mkfsArgs []string) error
	IsMounted(ctx context.Context, target string) (bool, error)
	Mount(ctx context.Context, source, target, fsType string, opts ...string) error
	Unmount(ctx context.Context, path string) error
	Statistics(volumePath string) (VolumeStatistics, error)
	GetDeviceByID(ctx context.Context, ID string) (string, error)
	GetDeviceLastPartition(ctx context.Context, source string) (string, error)
}
