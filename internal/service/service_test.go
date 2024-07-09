package service_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/UpCloudLtd/upcloud-csi/internal/service"
	"github.com/UpCloudLtd/upcloud-csi/internal/service/mock"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/client"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/request"
	upsvc "github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpCloudService_ListStorage(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `
		{
			"storages" : {
			   "storage" : [
					{
						"access" : "private",
						"state" : "online",
						"type" : "normal",
						"uuid" : "id1",
						"zone" : "fi-hel2"
					},
					{
						"access" : "private",
						"state" : "online",
						"type" : "normal",
						"uuid" : "id2",
						"zone" : "fi-hel2"
					},		
					{
						"access" : "private",
						"state" : "online",
						"type" : "backup",
						"uuid" : "id3",
						"zone" : "fi-hel2"
					},
					{
						"access" : "private",
						"state" : "online",
						"type" : "normal",
						"uuid" : "id4",
						"zone" : "fi-hel1"
					}
			   ]
			}
		 }
		`)
	}))
	defer srv.Close()
	c := service.NewUpCloudService(upsvc.New(client.New("", "", client.WithBaseURL(srv.URL))))
	storages, err := c.ListStorage(context.Background(), "fi-hel2")
	if err != nil {
		t.Error(err)
	}
	want := []*upcloud.Storage{
		{
			State: "online",
			Type:  "normal",
			UUID:  "id1",
			Zone:  "fi-hel2",
		},
		{
			State: "online",
			Type:  "normal",
			UUID:  "id2",
			Zone:  "fi-hel2",
		},
	}
	if len(want) != len(storages) {
		t.Errorf("storages len mismatch want %d got %d", len(want), len(storages))
		return
	}
	for i, s := range storages {
		w := want[i]
		if s.State != w.State {
			t.Errorf("storages[%d] invalid state want %s got %s", i, w.State, s.State)
		}
		if s.UUID != w.UUID {
			t.Errorf("storages[%d] invalid UUID want %s got %s", i, w.UUID, s.UUID)
		}
	}
}

func TestUpCloudService_ListStorageBackups(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `
		{
			"storages" : {
			   "storage" : [
					{
						"access" : "private",
						"state" : "online",
						"type" : "backup",
						"uuid" : "id1",
						"zone" : "fi-hel2",
						"origin": "id3"
					},
					{
						"access" : "private",
						"state" : "online",
						"type" : "backup",
						"uuid" : "id2",
						"zone" : "fi-hel2",
						"origin" : "id1"
					},
					{
						"access" : "private",
						"state" : "maintenance",
						"type" : "backup",
						"uuid" : "id3",
						"zone" : "fi-hel2"
					},
					{
						"access" : "private",
						"state" : "online",
						"type" : "backup",
						"uuid" : "id4",
						"zone" : "fi-hel2",
						"origin" : "id1"
					}
			   ]
			}
		 }
		`)
	}))
	defer srv.Close()

	c := service.NewUpCloudService(upsvc.New(client.New("", "", client.WithBaseURL(srv.URL))))
	storages, err := c.ListStorageBackups(context.Background(), "id1")
	assert.NoError(t, err)
	want := []*upcloud.Storage{
		{
			State:  "online",
			Type:   "normal",
			UUID:   "id2",
			Zone:   "fi-hel2",
			Origin: "id1",
		},
		{
			State:  "online",
			Type:   "normal",
			UUID:   "id4",
			Zone:   "fi-hel2",
			Origin: "id1",
		},
	}
	assert.Len(t, storages, len(want))
	for i, s := range storages {
		w := want[i]
		if s.State != w.State {
			t.Errorf("storages[%d] invalid state want %s got %s", i, w.State, s.State)
		}
		if s.UUID != w.UUID {
			t.Errorf("storages[%d] invalid UUID want %s got %s", i, w.UUID, s.UUID)
		}
	}

	storages, err = c.ListStorageBackups(context.Background(), "")
	assert.NoError(t, err)
	assert.Len(t, storages, 3)
}

func TestUpCloudService_AttachDetachStorage_Concurrency(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client := &mock.UpCloudClient{}
	s := service.NewUpCloudService(client)
	c := 10

	var wg sync.WaitGroup
	for i := 0; i < c; i++ {
		wg.Add(1)
		// populate backend with two nodes and add 5 storages per node
		serverUUID := fmt.Sprintf("test-node-%d", i%2)
		volUUID := fmt.Sprintf("test-vol-%d", i)
		client.StoreServer(&upcloud.ServerDetails{
			Server: upcloud.Server{
				UUID:  serverUUID,
				State: upcloud.ServerStateStarted,
			},
			StorageDevices: make([]upcloud.ServerStorageDevice, 0),
		})
		go func(volUUID, serverUUID string) {
			defer wg.Done()
			t1 := time.Now()
			err := s.AttachStorage(ctx, volUUID, serverUUID)
			t.Logf("attached %s to node %s in %s", volUUID, serverUUID, time.Since(t1))
			assert.NoError(t, err)
		}(volUUID, serverUUID)
	}
	wg.Wait()
	servers, err := client.GetServers(ctx)
	require.NoError(t, err)
	require.Len(t, servers.Servers, 2)
	for _, srv := range servers.Servers {
		d, err := client.GetServerDetails(ctx, &request.GetServerDetailsRequest{UUID: srv.UUID})
		if !assert.NoError(t, err) {
			continue
		}
		for _, storage := range d.StorageDevices {
			wg.Add(1)
			go func(volUUID, serverUUID string) {
				defer wg.Done()
				t1 := time.Now()
				err := s.DetachStorage(ctx, volUUID, serverUUID)
				t.Logf("detached %s from node %s in %s", volUUID, serverUUID, time.Since(t1))
				assert.NoError(t, err)
			}(storage.UUID, d.UUID)
		}
	}
	wg.Wait()
}
