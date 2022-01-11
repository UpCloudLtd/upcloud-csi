package driver

import (
	"context"
	"fmt"

	"github.com/UpCloudLtd/upcloud-go-api/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/upcloud/request"
	service "github.com/UpCloudLtd/upcloud-go-api/upcloud/service"
)

type upcloudClient struct {
	svc *service.Service
}

func (u *upcloudClient) GetStorageByUUID(ctx context.Context, storageUUID string) ([]*upcloud.StorageDetails, error) {
	gsr := &request.GetStoragesRequest{}
	storages, err := u.svc.GetStorages(gsr)
	if err != nil {
		return nil, err
	}
	volumes := make([]*upcloud.StorageDetails, 0)
	for _, s := range storages.Storages {
		fmt.Printf("%+v\n", s)
		if s.UUID == storageUUID {
			sd, _ := u.svc.GetStorageDetails(&request.GetStorageDetailsRequest{UUID: s.UUID})
			volumes = append(volumes, sd)
		}
	}
	return volumes, nil
}

func (u *upcloudClient) GetStorageByName(ctx context.Context, storageName string) ([]*upcloud.StorageDetails, error) {
	gsr := &request.GetStoragesRequest{}
	storages, err := u.svc.GetStorages(gsr)
	if err != nil {
		return nil, err
	}
	volumes := make([]*upcloud.StorageDetails, 0)
	for _, s := range storages.Storages {
		fmt.Printf("%+v\n", s)
		if s.Title == storageName {
			sd, _ := u.svc.GetStorageDetails(&request.GetStorageDetailsRequest{UUID: s.UUID})
			volumes = append(volumes, sd)
		}
	}
	return volumes, nil
}

func (u *upcloudClient) CreateStorage(ctx context.Context, csr *request.CreateStorageRequest) (*upcloud.StorageDetails, error) {
	s, err := u.svc.CreateStorage(csr)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	return s, nil
}

func (u *upcloudClient) DeleteStorage(ctx context.Context, storageUUID string) error {
	var err error
	volumes, err := u.GetStorageByUUID(ctx, storageUUID)
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
