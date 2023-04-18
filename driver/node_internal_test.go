package driver

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVolumeIDToDiskID(t *testing.T) {
	t.Parallel()
	volID := "f67db1ca-825b-40aa-a6f4-390ac6ff1b91"
	want := "virtio-f67db1ca825b40aaa6f4"
	got, err := volumeIDToDiskID(volID)
	require.NoError(t, err)
	if want != got {
		t.Errorf("volumeIDToDiskID('%s') failed want %s got %s", volID, want, got)
	}
}

func TestGetBlockDeviceByDiskID(t *testing.T) {
	t.Parallel()
	tempDir, err := os.MkdirTemp(os.TempDir(), fmt.Sprintf("test-%s-*", DefaultDriverName))
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
	t.Logf("using temp dir %s", tempDir)

	tempDevPath := filepath.Join(tempDir, "dev")
	t.Logf("using dev path %s", tempDevPath)

	idPath := filepath.Join(tempDir, udevDiskByIDPath)
	t.Logf("using disk id path %s", idPath)

	if err := os.MkdirAll(idPath, os.ModePerm); err != nil {
		t.Fatal(err)
	}

	// Test relative path
	vda, err := createTempFile(tempDevPath, "vda")
	require.NoError(t, err)

	vdaUUID := uuid.NewString()
	diskID, err := volumeIDToDiskID(vdaUUID)
	require.NoError(t, err)

	vdaSymLink := filepath.Join(idPath, diskID)

	// using ln command instead of Go's built-in so that link has relative path
	if err := exec.Command("ln", "-s", fmt.Sprintf("../../%s", filepath.Base(vda)), vdaSymLink).Run(); err != nil { //nolint: gosec // test
		t.Fatal(err)
	}

	want := vda
	got, err := getBlockDeviceByDiskID(context.TODO(), vdaSymLink)
	require.NoError(t, err)
	assert.Equal(t, want, got)

	// Test absolute path
	vdb, _ := createTempFile(tempDevPath, "vdb")
	vdbUUID := uuid.NewString()
	diskID, err = volumeIDToDiskID(vdbUUID)
	require.NoError(t, err)
	vdbSymLink := filepath.Join(idPath, diskID)
	if err := os.Symlink(vdb, vdbSymLink); err != nil {
		t.Fatal(err)
	}
	want = vdb
	got, err = getBlockDeviceByDiskID(context.TODO(), vdbSymLink)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestNodeExpandVolume(t *testing.T) {
	t.Parallel()
	d := NewMockDriver(nil)
	if _, err := d.NodeExpandVolume(context.TODO(), nil); err == nil {
		t.Error("NodeExpandVolume should return error. Only offline volume expansion is supported and it's handled by controller.")
	}
}

func createTempFile(dir, pattern string) (string, error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	return f.Name(), f.Close()
}
