package driver

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/UpCloudLtd/upcloud-go-api/v6/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v6/upcloud/client"
	upsvc "github.com/UpCloudLtd/upcloud-go-api/v6/upcloud/service"
	"github.com/stretchr/testify/assert"
)

func TestUpCloudService_listStorage(t *testing.T) {
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
	c := upCloudService{upsvc.New(client.New("", "", client.WithBaseURL(srv.URL)))}
	storages, err := c.listStorage(context.Background(), "fi-hel2")
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

func TestUpCloudService_listStorageBackups(t *testing.T) {
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

	c := upCloudService{upsvc.New(client.New("", "", client.WithBaseURL(srv.URL)))}
	storages, err := c.listStorageBackups(context.Background(), "id1")
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

	storages, err = c.listStorageBackups(context.Background(), "")
	assert.NoError(t, err)
	assert.Len(t, storages, 3)
}
