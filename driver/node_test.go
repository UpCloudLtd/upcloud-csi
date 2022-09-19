package driver

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestVolumeIDToDiskID(t *testing.T) {
	volID := "f67db1ca-825b-40aa-a6f4-390ac6ff1b91"
	want := "f67db1ca825b40aaa6f4"
	got := volumeIDToDiskID(volID)
	if want != got {
		t.Errorf("volumeIDToDiskID('%s') failed want %s got %s", volID, want, got)
	}
}

func TestGetDiskByID(t *testing.T) {
	tempDir, err := ioutil.TempDir(os.TempDir(), fmt.Sprintf("test-%s-*", DefaultDriverName))
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
	t.Logf("using temp dir %s", tempDir)

	devPath := filepath.Join(tempDir, "dev")
	t.Logf("using dev path %s", devPath)
	idPath := filepath.Join(tempDir, diskIDPath)
	t.Logf("using disk id path %s", idPath)
	if err := os.MkdirAll(idPath, os.ModePerm); err != nil {
		t.Fatal(err)
	}

	// Test relative path
	vda, _ := createTempFile(devPath, "vda")
	vdaUUID := uuid.NewString()
	vdaSymLink := filepath.Join(idPath, diskPrefix+volumeIDToDiskID(vdaUUID))

	// using ln command instead of Go's built-in so that link has relative path
	if err := exec.Command("ln", "-s", fmt.Sprintf("../../%s", filepath.Base(vda)), vdaSymLink).Run(); err != nil {
		t.Fatal(err)
	}

	want := vda
	got := getDiskByID(volumeIDToDiskID(vdaUUID), idPath)
	if got != want {
		t.Errorf("getDiskSource('%s') failed want %s got %s", vdaUUID, want, got)
	}

	// Test absolute path
	vdb, _ := createTempFile(devPath, "vdb")
	vdbUUID := uuid.NewString()
	vdbSymLink := filepath.Join(idPath, diskPrefix+volumeIDToDiskID(vdbUUID))
	if err := os.Symlink(vdb, vdbSymLink); err != nil {
		t.Fatal(err)
	}
	want = vdb
	got = getDiskByID(volumeIDToDiskID(vdbUUID), idPath)
	if got != want {
		t.Errorf("getDiskSource('%s') failed want %s got %s", vdbUUID, want, got)
	}
}

func createTempFile(dir, pattern string) (string, error) {
	f, err := ioutil.TempFile(dir, pattern)
	if err != nil {
		return "", err
	}
	return f.Name(), f.Close()
}

func TestNodeExpandVolume(t *testing.T) {
	d := NewMockDriver(nil)
	if _, err := d.NodeExpandVolume(context.TODO(), nil); err == nil {
		t.Error("NodeExpandVolume should return error. Only offline volume expansion is supported and it's handled by controller.")
	}
}
