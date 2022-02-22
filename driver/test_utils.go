package driver

import (
	"context"
	"fmt"
	"github.com/UpCloudLtd/upcloud-go-api/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/upcloud/request"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

type mockDriver struct {
	volumeExists bool
}

func NewMockDriver() *Driver {
	upcloudDriver := mockDriver{}

	socket := "/tmp/csi.sock"
	endpoint := "unix://" + socket

	return &Driver{
		options: &driverOptions{
			zone:     "demoRegion",
			endpoint: endpoint,
		},
		upclouddriver: &upcloudDriver,
		log:           logrus.New().WithField("test_enabled", true),
	}
}

func newMockStorage() *upcloud.Storage {
	id, _ := uuid.NewUUID()

	return &upcloud.Storage{
		Size: defaultVolumeSize,
		UUID: id.String(),
	}
}

func (m *mockDriver) Run() error {
	fmt.Println("sup")
	return nil
}

func (m *mockDriver) getStorageByUUID(ctx context.Context, storageUUID string) ([]*upcloud.StorageDetails, error) {

	return m.getStorageByName(ctx, storageUUID)
}

func (m *mockDriver) getStorageByName(ctx context.Context, storageName string) ([]*upcloud.StorageDetails, error) {
	if m.volumeExists {
		return nil, nil
	}

	s := []*upcloud.StorageDetails{
		{
			Storage: *newMockStorage(),
		},
	}
	return s, nil
}

func (m *mockDriver) createStorage(ctx context.Context, csr *request.CreateStorageRequest) (*upcloud.StorageDetails, error) {
	id, _ := uuid.NewUUID()
	s := &upcloud.StorageDetails{
		Storage:     *newMockStorage(),
		ServerUUIDs: upcloud.ServerUUIDSlice{id.String()}, // TODO change UUID prefix
	}

	return s, nil
}

func (m *mockDriver) deleteStorage(ctx context.Context, storageUUID string) error {
	return nil
}

func (m *mockDriver) attachStorage(ctx context.Context, storageUUID, serverUUID string) error {
	return nil
}

func (m *mockDriver) detachStorage(ctx context.Context, storageUUID, serverUUID string) error {
	return nil
}

func (m *mockDriver) listStorage(ctx context.Context, zone string) ([]*upcloud.Storage, error) {
	return []*upcloud.Storage{
		newMockStorage(),
		newMockStorage(),
	}, nil
}

func (m *mockDriver) getServer(ctx context.Context, uuid string) (*upcloud.ServerDetails, error) {
	return &upcloud.ServerDetails{}, nil
}

func (m *mockDriver) getServerByHostname(ctx context.Context, hostname string) (*upcloud.Server, error) {
	id, _ := uuid.NewUUID()
	return &upcloud.Server{UUID: id.String()}, nil
}
