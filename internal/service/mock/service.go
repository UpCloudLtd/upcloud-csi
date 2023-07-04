package mock

import (
	"context"

	"github.com/UpCloudLtd/upcloud-csi/internal/service"
	"github.com/UpCloudLtd/upcloud-go-api/v6/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v6/upcloud/request"
	"github.com/google/uuid"
)

type UpCloudServiceMock struct {
	VolumeNameExists bool
	VolumeUUIDExists bool
	CloneStorageSize int
	StorageSize      int
	StorageBackingUp bool

	SourceVolumeID string
}

func newMockStorage(size int, label ...upcloud.Label) *upcloud.Storage {
	id, _ := uuid.NewUUID()

	return &upcloud.Storage{
		Size:   size,
		UUID:   id.String(),
		Labels: label,
	}
}

func newMockBackupStorage(s *upcloud.Storage) *upcloud.Storage {
	b := newMockStorage(s.Size)
	b.Type = upcloud.StorageTypeBackup
	b.Origin = s.UUID
	return b
}

func (m *UpCloudServiceMock) GetStorageByUUID(ctx context.Context, storageUUID string) (*upcloud.StorageDetails, error) {
	if !m.VolumeUUIDExists {
		return nil, service.ErrStorageNotFound
	}

	s := &upcloud.StorageDetails{
		Storage: *newMockStorage(m.StorageSize),
	}
	return s, nil
}

func (m *UpCloudServiceMock) GetStorageByName(ctx context.Context, storageName string) ([]*upcloud.StorageDetails, error) {
	if !m.VolumeNameExists {
		return nil, nil
	}

	s := []*upcloud.StorageDetails{
		{
			Storage: *newMockStorage(m.StorageSize),
		},
	}
	return s, nil
}

func (m *UpCloudServiceMock) CreateStorage(ctx context.Context, csr *request.CreateStorageRequest) (*upcloud.StorageDetails, error) {
	id, _ := uuid.NewUUID()
	s := &upcloud.StorageDetails{
		Storage:     *newMockStorage(m.StorageSize),
		ServerUUIDs: upcloud.ServerUUIDSlice{id.String()}, // TODO change UUID prefix
	}

	return s, nil
}

func (m *UpCloudServiceMock) CloneStorage(ctx context.Context, csr *request.CloneStorageRequest, label ...upcloud.Label) (*upcloud.StorageDetails, error) {
	id, _ := uuid.NewUUID()
	s := &upcloud.StorageDetails{
		Storage:     *newMockStorage(m.CloneStorageSize, label...),
		ServerUUIDs: upcloud.ServerUUIDSlice{id.String()}, // TODO change UUID prefix
	}

	return s, nil
}

func (m *UpCloudServiceMock) DeleteStorage(ctx context.Context, storageUUID string) error {
	return nil
}

func (m *UpCloudServiceMock) AttachStorage(ctx context.Context, storageUUID, serverUUID string) error {
	return nil
}

func (m *UpCloudServiceMock) DetachStorage(ctx context.Context, storageUUID, serverUUID string) error {
	return nil
}

func (m *UpCloudServiceMock) ListStorage(ctx context.Context, zone string) ([]upcloud.Storage, error) {
	return []upcloud.Storage{
		*newMockStorage(m.StorageSize),
		*newMockStorage(m.StorageSize),
	}, nil
}

func (m *UpCloudServiceMock) GetServerByHostname(ctx context.Context, hostname string) (*upcloud.ServerDetails, error) {
	id, _ := uuid.NewUUID()
	return &upcloud.ServerDetails{
		Server: upcloud.Server{
			UUID: id.String(),
		},
	}, nil
}

func (m *UpCloudServiceMock) ResizeStorage(ctx context.Context, _ string, newSize int, deleteBackup bool) (*upcloud.StorageDetails, error) {
	id, _ := uuid.NewUUID()
	return &upcloud.StorageDetails{Storage: upcloud.Storage{UUID: id.String(), Size: newSize}}, nil
}

func (m *UpCloudServiceMock) ResizeBlockDevice(ctx context.Context, _ string, newSize int) (*upcloud.StorageDetails, error) {
	id, _ := uuid.NewUUID()
	return &upcloud.StorageDetails{Storage: upcloud.Storage{UUID: id.String(), Size: newSize}}, nil
}

func (m *UpCloudServiceMock) CreateStorageBackup(ctx context.Context, uuid, title string) (*upcloud.StorageDetails, error) {
	if m.StorageBackingUp {
		return nil, service.ErrBackupInProgress
	}
	s := newMockStorage(m.StorageSize)
	s.UUID = uuid
	s = newMockBackupStorage(s)
	s.Title = title

	return &upcloud.StorageDetails{Storage: *s}, nil
}

func (m *UpCloudServiceMock) ListStorageBackups(ctx context.Context, uuid string) ([]upcloud.Storage, error) {
	s := newMockStorage(m.StorageSize)
	return []upcloud.Storage{
		*newMockBackupStorage(s),
		*newMockBackupStorage(s),
	}, nil
}

func (m *UpCloudServiceMock) DeleteStorageBackup(ctx context.Context, uuid string) error {
	return nil
}

func (m *UpCloudServiceMock) GetStorageBackupByName(ctx context.Context, name string) (*upcloud.Storage, error) {
	var s *upcloud.Storage
	if !m.VolumeUUIDExists || name == "" {
		return nil, service.ErrStorageNotFound
	}
	s = newMockBackupStorage(newMockStorage(m.StorageSize))
	s.Title = name
	if m.SourceVolumeID != "" {
		s.Origin = m.SourceVolumeID
	}
	return s, nil
}

func (m *UpCloudServiceMock) RequireStorageOnline(ctx context.Context, s *upcloud.Storage) error {
	return nil
}
