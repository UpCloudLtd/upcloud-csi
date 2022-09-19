package driver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/UpCloudLtd/upcloud-go-api/v4/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v4/upcloud/request"
	"github.com/UpCloudLtd/upcloud-go-api/v4/upcloud/service"
)

// TODO sort out naming conventions

const (
	startServerTimeout  = 30
	stopServerTimeout   = 30
	storageStateTimeout = 3600 // 1h
)

var errUpCloudStorageNotFound = errors.New("upcloud: storage not found")

type upcloudClient struct {
	svc *service.ServiceContext
}

type upcloudService interface {
	getStorageByUUID(context.Context, string) (*upcloud.StorageDetails, error)
	getStorageByName(context.Context, string) ([]*upcloud.StorageDetails, error)
	createStorage(context.Context, *request.CreateStorageRequest) (*upcloud.StorageDetails, error)
	cloneStorage(context.Context, *request.CloneStorageRequest) (*upcloud.StorageDetails, error)
	deleteStorage(context.Context, string) error
	attachStorage(context.Context, string, string) error
	detachStorage(context.Context, string, string) error
	listStorage(context.Context, string) ([]upcloud.Storage, error)
	getServer(context.Context, string) (*upcloud.ServerDetails, error)
	getServerByHostname(context.Context, string) (*upcloud.Server, error)
	resizeStorage(ctx context.Context, uuid string, newSize int, deleteBackup bool) (*upcloud.StorageDetails, error)
	stopServer(ctx context.Context, uuid string) (*upcloud.ServerDetails, error)
	startServer(ctx context.Context, uuid string) (*upcloud.ServerDetails, error)
	getStorageBackupByName(context.Context, string) (*upcloud.Storage, error)
	createStorageBackup(ctx context.Context, uuid, title string) (*upcloud.StorageDetails, error)
	listStorageBackups(ctx context.Context, uuid string) ([]upcloud.Storage, error)
	deleteStorageBackup(ctx context.Context, uuid string) error
	waitForStorageState(ctx context.Context, uuid, state string) (*upcloud.StorageDetails, error)
}

func (u *upcloudClient) getStorageByUUID(ctx context.Context, storageUUID string) (*upcloud.StorageDetails, error) {
	gsr := &request.GetStoragesRequest{}
	storages, err := u.svc.GetStorages(ctx, gsr)
	if err != nil {
		return nil, err
	}
	for _, s := range storages.Storages {
		if s.UUID == storageUUID {
			return u.svc.GetStorageDetails(ctx, &request.GetStorageDetailsRequest{UUID: s.UUID})
		}
	}
	return nil, errUpCloudStorageNotFound
}

func (u *upcloudClient) getStorageByName(ctx context.Context, storageName string) ([]*upcloud.StorageDetails, error) {
	gsr := &request.GetStoragesRequest{}
	storages, err := u.svc.GetStorages(ctx, gsr)
	if err != nil {
		return nil, err
	}
	volumes := make([]*upcloud.StorageDetails, 0)
	for _, s := range storages.Storages {
		if s.Title == storageName {
			sd, _ := u.svc.GetStorageDetails(ctx, &request.GetStorageDetailsRequest{UUID: s.UUID})
			volumes = append(volumes, sd)
		}
	}
	return volumes, nil
}

func (u *upcloudClient) createStorage(ctx context.Context, csr *request.CreateStorageRequest) (*upcloud.StorageDetails, error) {
	s, err := u.svc.CreateStorage(ctx, csr)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	return u.svc.WaitForStorageState(ctx, &request.WaitForStorageStateRequest{
		UUID:         s.Storage.UUID,
		DesiredState: upcloud.StorageStateOnline,
		Timeout:      storageStateTimeout * time.Second,
	})
}

func (u *upcloudClient) cloneStorage(ctx context.Context, r *request.CloneStorageRequest) (*upcloud.StorageDetails, error) {
	s, err := u.svc.CloneStorage(ctx, r)
	if err != nil {
		return nil, err
	}
	return u.svc.WaitForStorageState(ctx, &request.WaitForStorageStateRequest{
		UUID:         s.Storage.UUID,
		DesiredState: upcloud.StorageStateOnline,
		Timeout:      storageStateTimeout * time.Second,
	})
}

func (u *upcloudClient) deleteStorage(ctx context.Context, storageUUID string) error {
	var err error
	volume, err := u.getStorageByUUID(ctx, storageUUID)
	if err != nil {
		return err
	}

	dsr := &request.DeleteStorageRequest{UUID: volume.UUID}
	if err = u.svc.DeleteStorage(ctx, dsr); err != nil {
		return err
	}
	return nil
}

func (u *upcloudClient) attachStorage(ctx context.Context, storageUUID, serverUUID string) error {
	details, err := u.svc.AttachStorage(ctx, &request.AttachStorageRequest{ServerUUID: serverUUID, StorageUUID: storageUUID, Address: "virtio"})
	if err != nil {
		return err
	}

	for _, s := range details.StorageDevices {
		if storageUUID == s.UUID {
			return nil
		}
	}
	return fmt.Errorf("storage device not found after attaching to server")
}

func (u *upcloudClient) detachStorage(ctx context.Context, storageUUID, serverUUID string) error {
	sd, err := u.svc.GetServerDetails(ctx, &request.GetServerDetailsRequest{UUID: serverUUID})
	if err != nil {
		return err
	}

	for _, device := range sd.StorageDevices {
		if device.UUID == storageUUID {
			details, err := u.svc.DetachStorage(ctx, &request.DetachStorageRequest{ServerUUID: serverUUID, Address: device.Address})
			if err != nil {
				return err
			}
			for _, s := range details.StorageDevices {
				if storageUUID == s.UUID {
					return fmt.Errorf("storage device still attached")
				}
			}
			return nil
		}
	}
	return fmt.Errorf("this shouldnt happen. serverUUID %s storageUUID %s context: %+v", serverUUID, storageUUID, sd)
}

func (u *upcloudClient) listStorage(ctx context.Context, zone string) ([]upcloud.Storage, error) {
	storages, err := u.svc.GetStorages(ctx, &request.GetStoragesRequest{Access: upcloud.StorageAccessPrivate})
	if err != nil {
		return nil, err
	}
	zoneStorage := make([]upcloud.Storage, 0)
	for _, s := range storages.Storages {
		if s.Zone == zone && s.Type == upcloud.StorageTypeNormal {
			zoneStorage = append(zoneStorage, s)
		}
	}
	return zoneStorage, nil
}

func (u *upcloudClient) getServer(ctx context.Context, uuid string) (*upcloud.ServerDetails, error) {
	r := request.GetServerDetailsRequest{
		UUID: uuid,
	}
	server, err := u.svc.GetServerDetails(ctx, &r)
	if err != nil {
		return nil, err
	}
	return server, nil
}

func (u *upcloudClient) getServerByHostname(ctx context.Context, hostname string) (*upcloud.Server, error) {
	servers, err := u.svc.GetServers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch servers: %s", err)
	}

	for _, server := range servers.Servers {
		if server.Hostname == hostname {
			return &server, nil
		}
	}

	return nil, fmt.Errorf("server with such hostname does not exist")
}

func (u *upcloudClient) resizeStorage(ctx context.Context, uuid string, newSize int, deleteBackup bool) (*upcloud.StorageDetails, error) {
	storage, err := u.svc.ModifyStorage(ctx, &request.ModifyStorageRequest{
		UUID: uuid,
		Size: newSize,
	})
	if err != nil {
		return nil, err
	}

	backup, err := u.svc.ResizeStorageFilesystem(ctx, &request.ResizeStorageFilesystemRequest{UUID: uuid})
	if err != nil {
		return nil, err
	}

	fmt.Printf("backup: %#v\n", *backup)

	if deleteBackup {
		if err = u.svc.DeleteStorage(ctx, &request.DeleteStorageRequest{UUID: backup.UUID}); err != nil {
			return nil, err
		}
	}

	return u.svc.WaitForStorageState(ctx, &request.WaitForStorageStateRequest{
		UUID:         storage.Storage.UUID,
		DesiredState: upcloud.StorageStateOnline,
		Timeout:      storageStateTimeout * time.Second,
	})
}

func (u *upcloudClient) stopServer(ctx context.Context, uuid string) (*upcloud.ServerDetails, error) {
	server, err := u.svc.StopServer(ctx, &request.StopServerRequest{
		UUID:    uuid,
		Timeout: stopServerTimeout,
	})
	if err != nil {
		return nil, err
	}
	return server, nil
}

func (u *upcloudClient) startServer(ctx context.Context, uuid string) (*upcloud.ServerDetails, error) {
	server, err := u.svc.StartServer(ctx, &request.StartServerRequest{
		UUID:    uuid,
		Timeout: startServerTimeout,
	})
	if err != nil {
		return nil, err
	}
	return server, nil
}

func (u *upcloudClient) createStorageBackup(ctx context.Context, uuid, title string) (*upcloud.StorageDetails, error) {
	backup, err := u.svc.CreateBackup(ctx, &request.CreateBackupRequest{
		UUID:  uuid,
		Title: title,
	})
	if err != nil {
		return nil, err
	}
	return u.svc.WaitForStorageState(ctx, &request.WaitForStorageStateRequest{
		UUID:         backup.UUID,
		DesiredState: upcloud.StorageStateOnline,
		Timeout:      storageStateTimeout * time.Second,
	})
}

func (u *upcloudClient) listStorageBackups(ctx context.Context, uuid string) ([]upcloud.Storage, error) {
	storages, err := u.svc.GetStorages(ctx, &request.GetStoragesRequest{Type: upcloud.StorageTypeBackup})
	if err != nil {
		return nil, err
	}
	backups := make([]upcloud.Storage, 0)
	for _, b := range storages.Storages {
		if b.Origin == uuid && b.State == upcloud.StorageStateOnline {
			backups = append(backups, b)
		}
	}
	return backups, nil
}

func (u *upcloudClient) deleteStorageBackup(ctx context.Context, uuid string) error {
	s, err := u.svc.GetStorageDetails(ctx, &request.GetStorageDetailsRequest{UUID: uuid})
	if err != nil {
		return err
	}
	if s.Type != upcloud.StorageTypeBackup {
		return fmt.Errorf("unable to delete storage backup '%s' (%s) has invalid type '%s'", s.Title, s.UUID, s.Type)
	}
	return u.svc.DeleteStorage(ctx, &request.DeleteStorageRequest{UUID: s.UUID})
}

func (u *upcloudClient) getStorageBackupByName(ctx context.Context, name string) (*upcloud.Storage, error) {
	storages, err := u.svc.GetStorages(ctx, &request.GetStoragesRequest{Type: upcloud.StorageTypeBackup})
	if err != nil {
		return nil, err
	}
	for _, s := range storages.Storages {
		if s.Title == name {
			return &s, nil
		}
	}
	return nil, errUpCloudStorageNotFound
}

func (u *upcloudClient) waitForStorageState(ctx context.Context, uuid, state string) (*upcloud.StorageDetails, error) {
	return u.svc.WaitForStorageState(ctx, &request.WaitForStorageStateRequest{
		UUID:         uuid,
		DesiredState: state,
		Timeout:      storageStateTimeout * time.Second,
	})
}
