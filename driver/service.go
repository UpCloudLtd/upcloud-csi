package driver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/UpCloudLtd/upcloud-go-api/v6/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v6/upcloud/request"
	upsvc "github.com/UpCloudLtd/upcloud-go-api/v6/upcloud/service"
)

const (
	storageStateTimeout time.Duration = time.Millisecond
)

var (
	errUpCloudStorageNotFound       = errors.New("upcloud: storage not found")
	errUpCloudServerNotFound        = errors.New("upcloud: server not found")
	errUpCloudServerStorageNotFound = errors.New("upcloud: server storage not found")
)

type service interface { //nolint:interfacebloat // Split this to smaller piece when it makes sense code wise
	getServerByHostname(context.Context, string) (*upcloud.ServerDetails, error)
	getStorageByUUID(context.Context, string) (*upcloud.StorageDetails, error)
	getStorageByName(context.Context, string) ([]*upcloud.StorageDetails, error)
	listStorage(context.Context, string) ([]upcloud.Storage, error)
	getStorageBackupByName(context.Context, string) (*upcloud.Storage, error)
	listStorageBackups(ctx context.Context, uuid string) ([]upcloud.Storage, error)
	requireStorageOnline(ctx context.Context, s *upcloud.Storage) error
	createStorage(context.Context, *request.CreateStorageRequest) (*upcloud.StorageDetails, error)
	cloneStorage(context.Context, *request.CloneStorageRequest, ...upcloud.Label) (*upcloud.StorageDetails, error)
	deleteStorage(context.Context, string) error
	attachStorage(context.Context, string, string) error
	detachStorage(context.Context, string, string) error
	resizeStorage(ctx context.Context, uuid string, newSize int, deleteBackup bool) (*upcloud.StorageDetails, error)
	resizeBlockDevice(ctx context.Context, uuid string, newSize int) (*upcloud.StorageDetails, error)
	createStorageBackup(ctx context.Context, uuid, title string) (*upcloud.StorageDetails, error)
	deleteStorageBackup(ctx context.Context, uuid string) error
	checkIfBackingUp(ctx context.Context, storageUUID string) (bool, error)
}

type upCloudService struct {
	svc *upsvc.Service
}

func (u *upCloudService) getStorageByUUID(ctx context.Context, storageUUID string) (*upcloud.StorageDetails, error) {
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

func (u *upCloudService) getStorageByName(ctx context.Context, storageName string) ([]*upcloud.StorageDetails, error) {
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

func (u *upCloudService) createStorage(ctx context.Context, csr *request.CreateStorageRequest) (*upcloud.StorageDetails, error) {
	s, err := u.svc.CreateStorage(ctx, csr)
	if err != nil {
		return nil, err
	}
	return u.svc.WaitForStorageState(ctx, &request.WaitForStorageStateRequest{
		UUID:         s.Storage.UUID,
		DesiredState: upcloud.StorageStateOnline,
		Timeout:      storageStateTimeout,
	})
}

func (u *upCloudService) cloneStorage(ctx context.Context, r *request.CloneStorageRequest, label ...upcloud.Label) (*upcloud.StorageDetails, error) {
	s, err := u.svc.CloneStorage(ctx, r)
	if err != nil {
		return nil, err
	}
	s, err = u.svc.WaitForStorageState(ctx, &request.WaitForStorageStateRequest{
		UUID:         s.Storage.UUID,
		DesiredState: upcloud.StorageStateOnline,
		Timeout:      storageStateTimeout,
	})
	if err != nil {
		return s, err
	}
	if len(label) > 0 {
		s, err = u.svc.ModifyStorage(ctx, &request.ModifyStorageRequest{
			UUID:   s.Storage.UUID,
			Labels: &label,
		})
		if err != nil {
			return s, err
		}
		s, err = u.svc.WaitForStorageState(ctx, &request.WaitForStorageStateRequest{
			UUID:         s.Storage.UUID,
			DesiredState: upcloud.StorageStateOnline,
			Timeout:      storageStateTimeout,
		})
	}
	return s, err
}

func (u *upCloudService) deleteStorage(ctx context.Context, storageUUID string) error {
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

func (u *upCloudService) attachStorage(ctx context.Context, storageUUID, serverUUID string) error {
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

func (u *upCloudService) detachStorage(ctx context.Context, storageUUID, serverUUID string) error {
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
	return errUpCloudServerStorageNotFound
}

func (u *upCloudService) listStorage(ctx context.Context, zone string) ([]upcloud.Storage, error) {
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

func (u *upCloudService) getServerByHostname(ctx context.Context, hostname string) (*upcloud.ServerDetails, error) {
	servers, err := u.svc.GetServers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch servers: %w", err)
	}

	for _, server := range servers.Servers {
		if server.Hostname == hostname {
			return u.svc.GetServerDetails(ctx, &request.GetServerDetailsRequest{
				UUID: server.UUID,
			})
		}
	}

	return nil, errUpCloudServerNotFound
}

func (u *upCloudService) resizeStorage(ctx context.Context, uuid string, newSize int, deleteBackup bool) (*upcloud.StorageDetails, error) {
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

	if deleteBackup {
		if err = u.svc.DeleteStorage(ctx, &request.DeleteStorageRequest{UUID: backup.UUID}); err != nil {
			return nil, err
		}
	}

	return u.svc.WaitForStorageState(ctx, &request.WaitForStorageStateRequest{
		UUID:         storage.Storage.UUID,
		DesiredState: upcloud.StorageStateOnline,
		Timeout:      storageStateTimeout,
	})
}

func (u *upCloudService) resizeBlockDevice(ctx context.Context, uuid string, newSize int) (*upcloud.StorageDetails, error) {
	storage, err := u.svc.ModifyStorage(ctx, &request.ModifyStorageRequest{
		UUID: uuid,
		Size: newSize,
	})
	if err != nil {
		return nil, err
	}
	return u.svc.WaitForStorageState(ctx, &request.WaitForStorageStateRequest{
		UUID:         storage.Storage.UUID,
		DesiredState: upcloud.StorageStateOnline,
		Timeout:      storageStateTimeout,
	})
}

func (u *upCloudService) createStorageBackup(ctx context.Context, uuid, title string) (*upcloud.StorageDetails, error) {
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
		Timeout:      storageStateTimeout,
	})
}

// listStorageBackups lists strage backups. If `originUUID` is empty all backups are retured.
func (u *upCloudService) listStorageBackups(ctx context.Context, originUUID string) ([]upcloud.Storage, error) {
	storages, err := u.svc.GetStorages(ctx, &request.GetStoragesRequest{Type: upcloud.StorageTypeBackup})
	if err != nil {
		return nil, err
	}
	backups := make([]upcloud.Storage, 0)
	for _, b := range storages.Storages {
		if originUUID == "" && b.Origin != "" || originUUID != "" && b.Origin == originUUID {
			backups = append(backups, b)
		}
	}
	return backups, nil
}

func (u *upCloudService) deleteStorageBackup(ctx context.Context, uuid string) error {
	s, err := u.svc.GetStorageDetails(ctx, &request.GetStorageDetailsRequest{UUID: uuid})
	if err != nil {
		return err
	}
	if s.Type != upcloud.StorageTypeBackup {
		return fmt.Errorf("unable to delete storage backup '%s' (%s) has invalid type '%s'", s.Title, s.UUID, s.Type)
	}
	return u.svc.DeleteStorage(ctx, &request.DeleteStorageRequest{UUID: s.UUID})
}

func (u *upCloudService) getStorageBackupByName(ctx context.Context, name string) (*upcloud.Storage, error) {
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

func (u *upCloudService) requireStorageOnline(ctx context.Context, s *upcloud.Storage) error {
	if s.State != upcloud.StorageStateOnline {
		if _, err := u.waitForStorageState(ctx, s.UUID, upcloud.StorageStateOnline); err != nil {
			return err
		}
	}
	return nil
}

func (u *upCloudService) waitForStorageState(ctx context.Context, uuid, state string) (*upcloud.StorageDetails, error) {
	return u.svc.WaitForStorageState(ctx, &request.WaitForStorageStateRequest{
		UUID:         uuid,
		DesiredState: state,
		Timeout:      storageStateTimeout,
	})
}

func (u *upCloudService) checkIfBackingUp(ctx context.Context, storageUUID string) (bool, error) {
	storage, err := u.getStorageByUUID(ctx, storageUUID)
	if err != nil {
		return false, err
	}

	if storage.State == upcloud.StorageStateBackuping {
		return true, nil
	}

	return false, nil
}
