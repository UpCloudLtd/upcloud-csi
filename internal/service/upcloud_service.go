package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/UpCloudLtd/upcloud-go-api/v6/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v6/upcloud/client"
	"github.com/UpCloudLtd/upcloud-go-api/v6/upcloud/request"
	upsvc "github.com/UpCloudLtd/upcloud-go-api/v6/upcloud/service"
)

const (
	storageStateTimeout time.Duration = time.Hour
	serverStateTimeout  time.Duration = 15 * time.Minute

	// clientTimeout helps to tune for timeout on requests to UpCloud API. Measurement: seconds.
	clientTimeout time.Duration = 120 * time.Second
)

type upCloudClient interface {
	upsvc.Storage

	WaitForServerState(ctx context.Context, r *request.WaitForServerStateRequest) (*upcloud.ServerDetails, error)
	GetServers(ctx context.Context) (*upcloud.Servers, error)
	GetServerDetails(ctx context.Context, r *request.GetServerDetailsRequest) (*upcloud.ServerDetails, error)
}

type UpCloudService struct {
	client upCloudClient
}

func NewUpCloudService(svc upCloudClient) *UpCloudService {
	return &UpCloudService{client: svc}
}

func NewUpCloudServiceFromCredentials(username, password string) (*UpCloudService, error) {
	if username == "" {
		return nil, errors.New("UpCloud API username is missing")
	}
	if password == "" {
		return nil, errors.New("UpCloud API password is missing")
	}
	return NewUpCloudService(
		upsvc.New(client.New(username, password, client.WithTimeout(clientTimeout))),
	), nil
}

func (u *UpCloudService) GetStorageByUUID(ctx context.Context, storageUUID string) (*upcloud.StorageDetails, error) {
	gsr := &request.GetStoragesRequest{}
	storages, err := u.client.GetStorages(ctx, gsr)
	if err != nil {
		return nil, err
	}
	for _, s := range storages.Storages {
		if s.UUID == storageUUID {
			return u.client.GetStorageDetails(ctx, &request.GetStorageDetailsRequest{UUID: s.UUID})
		}
	}
	return nil, ErrStorageNotFound
}

func (u *UpCloudService) GetStorageByName(ctx context.Context, storageName string) ([]*upcloud.StorageDetails, error) {
	gsr := &request.GetStoragesRequest{}
	storages, err := u.client.GetStorages(ctx, gsr)
	if err != nil {
		return nil, err
	}
	volumes := make([]*upcloud.StorageDetails, 0)
	for _, s := range storages.Storages {
		if s.Title == storageName {
			sd, _ := u.client.GetStorageDetails(ctx, &request.GetStorageDetailsRequest{UUID: s.UUID})
			volumes = append(volumes, sd)
		}
	}
	return volumes, nil
}

func (u *UpCloudService) CreateStorage(ctx context.Context, csr *request.CreateStorageRequest) (*upcloud.StorageDetails, error) {
	s, err := u.client.CreateStorage(ctx, csr)
	if err != nil {
		return nil, err
	}
	return u.waitForStorageOnline(ctx, s.Storage.UUID)
}

func (u *UpCloudService) CloneStorage(ctx context.Context, r *request.CloneStorageRequest, label ...upcloud.Label) (*upcloud.StorageDetails, error) {
	s, err := u.client.CloneStorage(ctx, r)
	if err != nil {
		return nil, err
	}
	s, err = u.waitForStorageOnline(ctx, s.Storage.UUID)
	if err != nil {
		return s, err
	}
	if len(label) > 0 {
		s, err = u.client.ModifyStorage(ctx, &request.ModifyStorageRequest{
			UUID:   s.Storage.UUID,
			Labels: &label,
		})
		if err != nil {
			return s, err
		}
		s, err = u.waitForStorageOnline(ctx, s.Storage.UUID)
	}
	return s, err
}

func (u *UpCloudService) DeleteStorage(ctx context.Context, storageUUID string) error {
	var err error
	volume, err := u.GetStorageByUUID(ctx, storageUUID)
	if err != nil {
		return err
	}

	dsr := &request.DeleteStorageRequest{UUID: volume.UUID}
	if err = u.client.DeleteStorage(ctx, dsr); err != nil {
		return err
	}
	return nil
}

func (u *UpCloudService) AttachStorage(ctx context.Context, storageUUID, serverUUID string) error {
	if err := u.waitForServerOnline(ctx, serverUUID); err != nil {
		return fmt.Errorf("failed to attach storage, pre-condition failed: %w", err)
	}
	details, err := u.client.AttachStorage(ctx, &request.AttachStorageRequest{ServerUUID: serverUUID, StorageUUID: storageUUID, Address: "virtio"})
	if err != nil {
		return err
	}

	for _, s := range details.StorageDevices {
		if storageUUID == s.UUID {
			// wait until server is no longer in maintenance state
			return u.waitForServerOnline(ctx, serverUUID)
		}
	}

	return fmt.Errorf("storage device not found after attaching to server")
}

func (u *UpCloudService) DetachStorage(ctx context.Context, storageUUID, serverUUID string) error {
	sd, err := u.client.GetServerDetails(ctx, &request.GetServerDetailsRequest{UUID: serverUUID})
	if err != nil {
		return err
	}
	if err := u.waitForServerOnline(ctx, serverUUID); err != nil {
		return fmt.Errorf("failed to detach storage, pre-condition failed: %w", err)
	}
	for _, device := range sd.StorageDevices {
		if device.UUID == storageUUID {
			details, err := u.client.DetachStorage(ctx, &request.DetachStorageRequest{ServerUUID: serverUUID, Address: device.Address})
			if err != nil {
				return err
			}
			for _, s := range details.StorageDevices {
				if storageUUID == s.UUID {
					return fmt.Errorf("storage device still attached")
				}
			}
			// wait until server is no longer in maintenance state
			return u.waitForServerOnline(ctx, serverUUID)
		}
	}
	return ErrServerStorageNotFound
}

func (u *UpCloudService) ListStorage(ctx context.Context, zone string) ([]upcloud.Storage, error) {
	storages, err := u.client.GetStorages(ctx, &request.GetStoragesRequest{Access: upcloud.StorageAccessPrivate})
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

func (u *UpCloudService) GetServerByHostname(ctx context.Context, hostname string) (*upcloud.ServerDetails, error) {
	servers, err := u.client.GetServers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch servers: %w", err)
	}

	for _, server := range servers.Servers {
		if server.Hostname == hostname {
			return u.client.GetServerDetails(ctx, &request.GetServerDetailsRequest{
				UUID: server.UUID,
			})
		}
	}

	return nil, ErrServerNotFound
}

func (u *UpCloudService) ResizeStorage(ctx context.Context, uuid string, newSize int, deleteBackup bool) (*upcloud.StorageDetails, error) {
	storage, err := u.client.ModifyStorage(ctx, &request.ModifyStorageRequest{
		UUID: uuid,
		Size: newSize,
	})
	if err != nil {
		return nil, err
	}

	backup, err := u.client.ResizeStorageFilesystem(ctx, &request.ResizeStorageFilesystemRequest{UUID: uuid})
	if err != nil {
		return nil, err
	}

	if deleteBackup {
		if err = u.client.DeleteStorage(ctx, &request.DeleteStorageRequest{UUID: backup.UUID}); err != nil {
			return nil, err
		}
	}

	return u.waitForStorageOnline(ctx, storage.Storage.UUID)
}

func (u *UpCloudService) ResizeBlockDevice(ctx context.Context, uuid string, newSize int) (*upcloud.StorageDetails, error) {
	storage, err := u.client.ModifyStorage(ctx, &request.ModifyStorageRequest{
		UUID: uuid,
		Size: newSize,
	})
	if err != nil {
		return nil, err
	}
	return u.waitForStorageOnline(ctx, storage.Storage.UUID)
}

func (u *UpCloudService) CreateStorageBackup(ctx context.Context, uuid, title string) (*upcloud.StorageDetails, error) {
	// check that a backup creation is not currently in progress
	storage, err := u.GetStorageByUUID(ctx, uuid)
	if err != nil {
		return nil, err
	}

	if storage.State == upcloud.StorageStateBackuping {
		return nil, ErrBackupInProgress
	}

	backup, err := u.client.CreateBackup(ctx, &request.CreateBackupRequest{
		UUID:  uuid,
		Title: title,
	})
	if err != nil {
		return nil, err
	}
	return u.waitForStorageOnline(ctx, backup.UUID)
}

// listStorageBackups lists strage backups. If `originUUID` is empty all backups are retured.
func (u *UpCloudService) ListStorageBackups(ctx context.Context, originUUID string) ([]upcloud.Storage, error) {
	storages, err := u.client.GetStorages(ctx, &request.GetStoragesRequest{Type: upcloud.StorageTypeBackup})
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

func (u *UpCloudService) DeleteStorageBackup(ctx context.Context, uuid string) error {
	s, err := u.client.GetStorageDetails(ctx, &request.GetStorageDetailsRequest{UUID: uuid})
	if err != nil {
		return err
	}
	if s.Type != upcloud.StorageTypeBackup {
		return fmt.Errorf("unable to delete storage backup '%s' (%s) has invalid type '%s'", s.Title, s.UUID, s.Type)
	}
	return u.client.DeleteStorage(ctx, &request.DeleteStorageRequest{UUID: s.UUID})
}

func (u *UpCloudService) GetStorageBackupByName(ctx context.Context, name string) (*upcloud.Storage, error) {
	storages, err := u.client.GetStorages(ctx, &request.GetStoragesRequest{Type: upcloud.StorageTypeBackup})
	if err != nil {
		return nil, err
	}
	for _, s := range storages.Storages {
		if s.Title == name {
			return &s, nil
		}
	}
	return nil, ErrStorageNotFound
}

func (u *UpCloudService) RequireStorageOnline(ctx context.Context, s *upcloud.Storage) error {
	if s.State != upcloud.StorageStateOnline {
		if _, err := u.waitForStorageOnline(ctx, s.UUID); err != nil {
			return err
		}
	}
	return nil
}

func (u *UpCloudService) waitForStorageOnline(ctx context.Context, uuid string) (*upcloud.StorageDetails, error) {
	return u.client.WaitForStorageState(ctx, &request.WaitForStorageStateRequest{
		UUID:         uuid,
		DesiredState: upcloud.StorageStateOnline,
		Timeout:      storageStateTimeout,
	})
}

func (u *UpCloudService) waitForServerOnline(ctx context.Context, uuid string) error {
	_, err := u.client.WaitForServerState(ctx, &request.WaitForServerStateRequest{
		UUID:         uuid,
		DesiredState: upcloud.ServerStateStarted,
		Timeout:      serverStateTimeout,
	})
	return err
}
