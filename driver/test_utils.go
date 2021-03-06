package driver

import (
	"context"
	"fmt"
	"github.com/UpCloudLtd/upcloud-go-api/v4/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v4/upcloud/request"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"strings"
)

var (
	volCap = &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{},
		},
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		},
	}
)

type MockDriver struct {
	Driver
	options *driverOptions
	//
	//srv     *grpc.Server
	//httpSrv http.Server
	//
	mounter Mounter
	log     *logrus.Entry
	//
	//upcloudclient *upcloudservice.Service
	//upclouddriver upcloudService
	//
	//healthChecker *HealthChecker
	//
	//storage upcloud.Storage
	//// ready defines whether the driver is ready to function. This value will
	//// be used by the `Identity` service via the `Probe()` method.
	//readyMu sync.Mutex // protects ready
	//ready   bool
}

type mockUpCloudDriver struct {
	volumeExists bool
}

func NewMockDriver() *Driver {
	upcloudDriver := mockUpCloudDriver{}

	socket := "/tmp/csi.sock"
	endpoint := "unix://" + socket

	log := logrus.New().WithField("test_enabled", true)

	return &Driver{
		options: &driverOptions{
			zone:       "demoRegion",
			endpoint:   endpoint,
			volumeName: "device",
		},
		mounter:       newMounter(log),
		upclouddriver: &upcloudDriver,
		log:           log,
	}
}

func newMockStorage() *upcloud.Storage {
	id, _ := uuid.NewUUID()

	return &upcloud.Storage{
		Size: 10,
		UUID: id.String(),
	}
}

func (m *mockUpCloudDriver) Run() error {
	fmt.Println("sup")
	return nil
}

func (m *mockUpCloudDriver) getStorageByUUID(ctx context.Context, storageUUID string) ([]*upcloud.StorageDetails, error) {

	return m.getStorageByName(ctx, storageUUID)
}

func (m *mockUpCloudDriver) getStorageByName(ctx context.Context, storageName string) ([]*upcloud.StorageDetails, error) {
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

func (m *mockUpCloudDriver) createStorage(ctx context.Context, csr *request.CreateStorageRequest) (*upcloud.StorageDetails, error) {
	id, _ := uuid.NewUUID()
	s := &upcloud.StorageDetails{
		Storage:     *newMockStorage(),
		ServerUUIDs: upcloud.ServerUUIDSlice{id.String()}, // TODO change UUID prefix
	}

	return s, nil
}

func (m *mockUpCloudDriver) deleteStorage(ctx context.Context, storageUUID string) error {
	return nil
}

func (m *mockUpCloudDriver) attachStorage(ctx context.Context, storageUUID, serverUUID string) error {
	return nil
}

func (m *mockUpCloudDriver) detachStorage(ctx context.Context, storageUUID, serverUUID string) error {
	return nil
}

func (m *mockUpCloudDriver) listStorage(ctx context.Context, zone string) ([]*upcloud.Storage, error) {
	return []*upcloud.Storage{
		newMockStorage(),
		newMockStorage(),
	}, nil
}

func (m *mockUpCloudDriver) getServer(ctx context.Context, uuid string) (*upcloud.ServerDetails, error) {
	return &upcloud.ServerDetails{}, nil
}

func (m *mockUpCloudDriver) getServerByHostname(ctx context.Context, hostname string) (*upcloud.Server, error) {
	id, _ := uuid.NewUUID()
	return &upcloud.Server{UUID: id.String()}, nil
}

func (m *mockUpCloudDriver) resizeStorage(ctx context.Context, uuid_ string, newSize int) (*upcloud.StorageDetails, error) {
	id, _ := uuid.NewUUID()
	return &upcloud.StorageDetails{Storage: upcloud.Storage{UUID: id.String()}}, nil
}

func (m *mockUpCloudDriver) startServer(ctx context.Context, uuid string) (*upcloud.ServerDetails, error) {
	return nil, nil
}

func (m *mockUpCloudDriver) stopServer(ctx context.Context, uuid string) (*upcloud.ServerDetails, error) {
	return nil, nil
}

func (m *mockUpCloudDriver) getDiskSource(volumeID string) string {
	fullId := strings.Join(strings.Split(volumeID, "-"), "")
	if len(fullId) <= 20 {
		return ""
	}

	link, err := os.Readlink(filepath.Join(diskIDPath, diskPrefix+fullId[:20]))
	if err != nil {
		fmt.Println(fmt.Errorf("failed to get the link to source"))
		return ""
	}
	source := "/dev" + strings.TrimPrefix(link, "../..")

	return source
}
