package driver

import (
	"context"
	"net/url"

	"github.com/UpCloudLtd/upcloud-go-api/v6/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v6/upcloud/request"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

type MockDriver struct {
	Driver
}

type mockUpCloudService struct {
	volumeNameExists bool
	volumeUUIDExists bool
	cloneStorageSize int
	storageSize      int
}

func NewMockDriver(svc service) *Driver {
	if svc == nil {
		svc = &mockUpCloudService{storageSize: 10, cloneStorageSize: 10, volumeUUIDExists: true}
	}

	endpoint, _ := url.Parse("unix:///tmp/csi.sock")

	log := logrus.New().WithField("test_enabled", true)

	return &Driver{
		svc: svc,
		options: Options{
			Zone:     "demoRegion",
			Endpoint: endpoint,
		},
		log: log,
	}
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

func (m *mockUpCloudService) getStorageByUUID(ctx context.Context, storageUUID string) (*upcloud.StorageDetails, error) {
	if !m.volumeUUIDExists {
		return nil, errUpCloudStorageNotFound
	}

	s := &upcloud.StorageDetails{
		Storage: *newMockStorage(m.storageSize),
	}
	return s, nil
}

func (m *mockUpCloudService) getStorageByName(ctx context.Context, storageName string) ([]*upcloud.StorageDetails, error) {
	if !m.volumeNameExists {
		return nil, nil
	}

	s := []*upcloud.StorageDetails{
		{
			Storage: *newMockStorage(m.storageSize),
		},
	}
	return s, nil
}

func (m *mockUpCloudService) createStorage(ctx context.Context, csr *request.CreateStorageRequest) (*upcloud.StorageDetails, error) {
	id, _ := uuid.NewUUID()
	s := &upcloud.StorageDetails{
		Storage:     *newMockStorage(m.storageSize),
		ServerUUIDs: upcloud.ServerUUIDSlice{id.String()}, // TODO change UUID prefix
	}

	return s, nil
}

func (m *mockUpCloudService) cloneStorage(ctx context.Context, csr *request.CloneStorageRequest, label ...upcloud.Label) (*upcloud.StorageDetails, error) {
	id, _ := uuid.NewUUID()
	s := &upcloud.StorageDetails{
		Storage:     *newMockStorage(m.cloneStorageSize, label...),
		ServerUUIDs: upcloud.ServerUUIDSlice{id.String()}, // TODO change UUID prefix
	}

	return s, nil
}

func (m *mockUpCloudService) deleteStorage(ctx context.Context, storageUUID string) error {
	return nil
}

func (m *mockUpCloudService) attachStorage(ctx context.Context, storageUUID, serverUUID string) error {
	return nil
}

func (m *mockUpCloudService) detachStorage(ctx context.Context, storageUUID, serverUUID string) error {
	return nil
}

func (m *mockUpCloudService) listStorage(ctx context.Context, zone string) ([]upcloud.Storage, error) {
	return []upcloud.Storage{
		*newMockStorage(m.storageSize),
		*newMockStorage(m.storageSize),
	}, nil
}

func (m *mockUpCloudService) getServerByHostname(ctx context.Context, hostname string) (*upcloud.Server, error) {
	id, _ := uuid.NewUUID()
	return &upcloud.Server{UUID: id.String()}, nil
}

func (m *mockUpCloudService) resizeStorage(ctx context.Context, _ string, newSize int, deleteBackup bool) (*upcloud.StorageDetails, error) {
	id, _ := uuid.NewUUID()
	return &upcloud.StorageDetails{Storage: upcloud.Storage{UUID: id.String(), Size: newSize}}, nil
}

func (m *mockUpCloudService) resizeBlockDevice(ctx context.Context, _ string, newSize int) (*upcloud.StorageDetails, error) {
	id, _ := uuid.NewUUID()
	return &upcloud.StorageDetails{Storage: upcloud.Storage{UUID: id.String(), Size: newSize}}, nil
}

func (m *mockUpCloudService) createStorageBackup(ctx context.Context, uuid, title string) (*upcloud.StorageDetails, error) {
	s := newMockStorage(m.storageSize)
	s.UUID = uuid
	s = newMockBackupStorage(s)
	s.Title = title
	return &upcloud.StorageDetails{Storage: *s}, nil
}

func (m *mockUpCloudService) listStorageBackups(ctx context.Context, uuid string) ([]upcloud.Storage, error) {
	s := newMockStorage(m.storageSize)
	return []upcloud.Storage{
		*newMockBackupStorage(s),
		*newMockBackupStorage(s),
	}, nil
}

func (m *mockUpCloudService) deleteStorageBackup(ctx context.Context, uuid string) error {
	return nil
}

func (m *mockUpCloudService) getStorageBackupByName(ctx context.Context, name string) (*upcloud.Storage, error) {
	s := newMockBackupStorage(newMockStorage(m.storageSize))
	s.Title = name
	return s, nil
}

func (m *mockUpCloudService) requireStorageOnline(ctx context.Context, s *upcloud.Storage) error {
	return nil
}
