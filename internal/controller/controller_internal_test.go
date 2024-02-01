package controller

import (
	"testing"

	"github.com/UpCloudLtd/upcloud-go-api/v6/upcloud"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaginateStorage(t *testing.T) {
	t.Parallel()
	s := []upcloud.Storage{{UUID: "1"}, {UUID: "2"}}
	var next int

	t.Log("testing that empty start token and excessive size returns equal slice")
	want := s[1:]
	got, next := paginateStorage(want, 0, 10)
	assert.Equal(t, want, got)
	assert.Equal(t, 0, next)

	t.Log("testing that zero size returns empty slice")
	got, next = paginateStorage(s, 1, 0)
	assert.Equal(t, want, got)
	assert.Equal(t, 0, next)

	t.Log("testing that start overflow return equal slice and next token set to zero")
	want = s[2:]
	got, next = paginateStorage(s, 100, 1)
	assert.Equal(t, want, got)
	assert.Equal(t, 0, next)

	s = append(s,
		upcloud.Storage{UUID: "3"},
		upcloud.Storage{UUID: "4"},
		upcloud.Storage{UUID: "5"},
		upcloud.Storage{UUID: "6"},
		upcloud.Storage{UUID: "7"},
	)
	size := 1
	t.Logf("testing pagination with page size %d", size)
	next = 0
	for i := range s {
		got, next = paginateStorage(s, next, size)
		t.Logf("got page size %d and %d as next page", len(got), next)
		assert.Equal(t, s[i*size], got[0])
		if next < 1 {
			break
		}
	}
	size = 4
	next = 0
	t.Logf("testing pagination with page size %d", size)
	for i := range s {
		got, next = paginateStorage(s, next, size)
		t.Logf("got page size %d and %d as next page", len(got), next)
		assert.Equal(t, s[i*size], got[0])
		assert.LessOrEqual(t, len(got), size)
		if next < 1 {
			break
		}
	}
}

func TestParseToken(t *testing.T) {
	t.Parallel()
	want := 0
	got, err := parseToken("")
	assert.NoError(t, err)
	assert.Equal(t, want, got)

	want = 10
	got, err = parseToken("10")
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestIsValidUUID(t *testing.T) {
	t.Parallel()

	assert.False(t, isValidUUID(""))
	assert.False(t, isValidUUID("0160ffc3-58ec-4670-bdc9"))
	assert.True(t, isValidUUID("0160ffc3-58ec-4670-bdc9-27fe385d281d"))
}

func TestIsValidStorageUUID(t *testing.T) {
	t.Parallel()

	assert.True(t, isValidUUID("1160ffc3-58ec-4670-bdc9-27fe385d281d"))
	assert.True(t, isValidUUID("0160ffc3-58ec-4670-bdc9-27fe385d281d"))
}

func TestCreateVolumeRequestEncryptionAtRest(t *testing.T) {
	t.Parallel()

	require.False(t, createVolumeRequestEncryptionAtRest(&csi.CreateVolumeRequest{}))

	p := map[string]string{}
	require.False(t, createVolumeRequestEncryptionAtRest(&csi.CreateVolumeRequest{Parameters: p}))

	p["encryption"] = "data-at-restx"
	require.False(t, createVolumeRequestEncryptionAtRest(&csi.CreateVolumeRequest{Parameters: p}))

	p["encryption"] = ""
	require.False(t, createVolumeRequestEncryptionAtRest(&csi.CreateVolumeRequest{Parameters: p}))

	p["encryption"] = "data-at-rest"
	require.True(t, createVolumeRequestEncryptionAtRest(&csi.CreateVolumeRequest{Parameters: p}))
}
