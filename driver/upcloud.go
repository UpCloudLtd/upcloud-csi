package driver

import (
	"context"
	"fmt"

	"github.com/UpCloudLtd/upcloud-go-api/v4/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v4/upcloud/request"
	"github.com/UpCloudLtd/upcloud-go-api/v4/upcloud/service"
)

// TODO sort out naming conventions

const (
	startServerTimeout = 25
	stopServerTimeout  = 15
)

type upcloudClient struct {
	svc *service.Service
}

type upcloudService interface {
	getStorageByUUID(context.Context, string) ([]*upcloud.StorageDetails, error)
	getStorageByName(context.Context, string) ([]*upcloud.StorageDetails, error)
	createStorage(context.Context, *request.CreateStorageRequest) (*upcloud.StorageDetails, error)
	deleteStorage(context.Context, string) error
	attachStorage(context.Context, string, string) error
	detachStorage(context.Context, string, string) error
	listStorage(context.Context, string) ([]*upcloud.Storage, error)
	getServer(context.Context, string) (*upcloud.ServerDetails, error)
	getServerByHostname(context.Context, string) (*upcloud.Server, error)
	resizeStorage(ctx context.Context, uuid string, newSize int) (*upcloud.StorageDetails, error)
	stopServer(ctx context.Context, uuid string) (*upcloud.ServerDetails, error)
	startServer(ctx context.Context, uuid string) (*upcloud.ServerDetails, error)
}

func (u *upcloudClient) getStorageByUUID(ctx context.Context, storageUUID string) ([]*upcloud.StorageDetails, error) {
	gsr := &request.GetStoragesRequest{}
	storages, err := u.svc.GetStorages(gsr)
	if err != nil {
		return nil, err
	}
	volumes := make([]*upcloud.StorageDetails, 0)
	for _, s := range storages.Storages {
		if s.UUID == storageUUID {
			sd, _ := u.svc.GetStorageDetails(&request.GetStorageDetailsRequest{UUID: s.UUID})
			volumes = append(volumes, sd)
		}
	}
	return volumes, nil
}

func (u *upcloudClient) getStorageByName(ctx context.Context, storageName string) ([]*upcloud.StorageDetails, error) {
	gsr := &request.GetStoragesRequest{}
	storages, err := u.svc.GetStorages(gsr)
	if err != nil {
		return nil, err
	}
	volumes := make([]*upcloud.StorageDetails, 0)
	for _, s := range storages.Storages {
		if s.Title == storageName {
			sd, _ := u.svc.GetStorageDetails(&request.GetStorageDetailsRequest{UUID: s.UUID})
			volumes = append(volumes, sd)
		}
	}
	return volumes, nil
}

func (u *upcloudClient) createStorage(ctx context.Context, csr *request.CreateStorageRequest) (*upcloud.StorageDetails, error) {
	s, err := u.svc.CreateStorage(csr)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	return s, nil
}

func (u *upcloudClient) deleteStorage(ctx context.Context, storageUUID string) error {
	var err error
	volumes, err := u.getStorageByUUID(ctx, storageUUID)
	if err != nil {
		return err
	}

	for _, v := range volumes {
		dsr := &request.DeleteStorageRequest{UUID: v.UUID}
		err = u.svc.DeleteStorage(dsr)
	}
	if err != nil {
		return err
	}
	return nil
}

func (u *upcloudClient) attachStorage(ctx context.Context, storageUUID, serverUUID string) error {
	details, err := u.svc.AttachStorage(&request.AttachStorageRequest{ServerUUID: serverUUID, StorageUUID: storageUUID, Address: "virtio"})
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
	sd, err := u.svc.GetServerDetails(&request.GetServerDetailsRequest{UUID: serverUUID})
	if err != nil {
		return err
	}

	for _, device := range sd.StorageDevices {
		if device.UUID == storageUUID {
			details, err := u.svc.DetachStorage(&request.DetachStorageRequest{ServerUUID: serverUUID, Address: device.Address})
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

func (u *upcloudClient) listStorage(ctx context.Context, zone string) ([]*upcloud.Storage, error) {
	storages, err := u.svc.GetStorages(&request.GetStoragesRequest{Type: "normal", Access: "private"})
	if err != nil {
		return nil, err
	}
	zoneStorage := make([]*upcloud.Storage, 0)
	for _, s := range storages.Storages {
		if s.Zone == zone {
			zoneStorage = append(zoneStorage, &s)
		}
	}
	return zoneStorage, nil
}

func (u *upcloudClient) getServer(ctx context.Context, uuid string) (*upcloud.ServerDetails, error) {
	r := request.GetServerDetailsRequest{
		UUID: uuid,
	}
	server, err := u.svc.GetServerDetails(&r)
	if err != nil {
		return nil, err
	}
	return server, nil
}

func (u *upcloudClient) getServerByHostname(ctx context.Context, hostname string) (*upcloud.Server, error) {
	servers, err := u.svc.GetServers()
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

func (u *upcloudClient) resizeStorage(ctx context.Context, uuid string, newSize int) (*upcloud.StorageDetails, error) {
	storage, err := u.svc.ModifyStorage(&request.ModifyStorageRequest{
		UUID: uuid,
		Size: newSize,
	})
	if err != nil {
		return nil, err
	}

	backup, err := u.svc.ResizeStorageFilesystem(&request.ResizeStorageFilesystemRequest{UUID: uuid})
	if err != nil {
		return nil, err
	}

	fmt.Printf("backup: %#v\n", *backup)

	return storage, nil
}

func (u *upcloudClient) stopServer(ctx context.Context, uuid string) (*upcloud.ServerDetails, error) {
	server, err := u.svc.StopServer(&request.StopServerRequest{
		UUID:    uuid,
		Timeout: stopServerTimeout,
	})
	if err != nil {
		return nil, err
	}
	return server, nil
}

func (u *upcloudClient) startServer(ctx context.Context, uuid string) (*upcloud.ServerDetails, error) {
	server, err := u.svc.StartServer(&request.StartServerRequest{
		UUID:    uuid,
		Timeout: startServerTimeout,
	})
	if err != nil {
		return nil, err
	}
	return server, nil
}
