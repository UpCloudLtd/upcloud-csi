package mock

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/request"
	upsvc "github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/service"
)

type UpCloudClient struct {
	upsvc.Storage

	servers sync.Map
}

func (u *UpCloudClient) StoreServer(s *upcloud.ServerDetails) {
	u.servers.LoadOrStore(s.UUID, s)
}

func (u *UpCloudClient) getServer(id string) *upcloud.ServerDetails {
	if s, ok := u.servers.Load(id); ok {
		return s.(*upcloud.ServerDetails)
	}
	return nil
}

func (u *UpCloudClient) WaitForServerState(ctx context.Context, r *request.WaitForServerStateRequest) (*upcloud.ServerDetails, error) {
	s, _ := u.GetServerDetails(ctx, &request.GetServerDetailsRequest{
		UUID: r.UUID,
	})
	return s, nil
}

func (u *UpCloudClient) GetServers(ctx context.Context) (*upcloud.Servers, error) {
	s := []upcloud.Server{}
	u.servers.Range(func(key, value any) bool {
		if d, ok := value.(*upcloud.ServerDetails); ok {
			s = append(s, d.Server)
		}
		return true
	})
	return &upcloud.Servers{Servers: s}, nil
}

func (u *UpCloudClient) GetServerDetails(ctx context.Context, r *request.GetServerDetailsRequest) (*upcloud.ServerDetails, error) {
	if s := u.getServer(r.UUID); s != nil {
		return s, nil
	}
	return nil, fmt.Errorf("server '%s' not found", r.UUID)
}

func (u *UpCloudClient) AttachStorage(ctx context.Context, r *request.AttachStorageRequest) (*upcloud.ServerDetails, error) {
	server := u.getServer(r.ServerUUID)
	if server == nil {
		return server, errors.New("server not found")
	}
	if server.State != upcloud.ServerStateStarted {
		return nil, fmt.Errorf("server %s state is %s", r.ServerUUID, server.State)
	}
	server.State = upcloud.ServerStateMaintenance
	u.StoreServer(server)
	time.Sleep(time.Duration(rand.Intn(200)+100) * time.Millisecond) //nolint:gosec // using weak random number doesn't affect the result.
	server.State = upcloud.ServerStateStarted
	if server.StorageDevices == nil {
		server.StorageDevices = make(upcloud.ServerStorageDeviceSlice, 0)
	}
	server.StorageDevices = append(server.StorageDevices, upcloud.ServerStorageDevice{
		Address: fmt.Sprintf("%s:%d", r.Address, len(server.StorageDevices)+1),
		UUID:    r.StorageUUID,
		Size:    10,
	})
	u.StoreServer(server)

	return u.getServer(r.ServerUUID), nil
}

func (u *UpCloudClient) DetachStorage(ctx context.Context, r *request.DetachStorageRequest) (*upcloud.ServerDetails, error) {
	server := u.getServer(r.ServerUUID)
	if server == nil {
		return server, fmt.Errorf("server %s not found", r.ServerUUID)
	}
	if server.State != upcloud.ServerStateStarted {
		return nil, fmt.Errorf("server %s state is %s", r.ServerUUID, server.State)
	}
	server.State = upcloud.ServerStateMaintenance
	u.StoreServer(server)
	time.Sleep(time.Duration(rand.Intn(200)+100) * time.Millisecond) //nolint:gosec // using weak random number doesn't affect the result.
	server = u.getServer(r.ServerUUID)
	server.State = upcloud.ServerStateStarted
	if len(server.StorageDevices) > 0 {
		storage := make([]upcloud.ServerStorageDevice, 0)
		for i := range server.StorageDevices {
			if server.StorageDevices[i].Address != r.Address {
				storage = append(storage, server.StorageDevices[i])
			}
		}
		server.StorageDevices = storage
	}
	u.StoreServer(server)

	return server, nil
}
