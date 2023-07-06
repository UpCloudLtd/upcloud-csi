package filesystem

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const driverName = "storage.csi.upclous.com"

func TestSfdiskOutputGetLastPartition(t *testing.T) {
	t.Parallel()
	outputMultiple := `
		Device
		/dev/vda1
		/dev/vda2
		/dev/vda3
	`
	outputSingle := `
		Device
		/dev/vda1
	`
	outputNone := `
		Device
	`
	want := "/dev/vda3"
	got, _ := sfdiskOutputGetLastPartition("/dev/vda", outputMultiple)
	if want != got {
		t.Errorf("sfdiskOutputGetLastPartition failed want %s got %s", want, got)
	}

	want = "/dev/vda1"
	got, _ = sfdiskOutputGetLastPartition("/dev/vda", outputSingle)
	if want != got {
		t.Errorf("sfdiskOutputGetLastPartition failed want %s got %s", want, got)
	}

	want = ""
	got, _ = sfdiskOutputGetLastPartition("/dev/vda", outputNone)
	if want != got {
		t.Errorf("sfdiskOutputGetLastPartition failed want %s got %s", want, got)
	}
}

func TestFilesystemFilesystem(t *testing.T) {
	t.Parallel()
	if err := checkSystemRequirements(); err != nil {
		t.Skipf("skipping test: %s", err.Error())
	}
	// create 10MB fake partition
	part, err := createDeviceFile(1e7)
	if err != nil {
		t.Error(err)
		return
	}
	defer os.Remove(part)
	t.Logf("create fake partition %s", part)

	m := newTestFilesystem()

	if err := m.createFilesystem(context.Background(), part, "ext4", nil); err != nil {
		t.Errorf("Format failed with error: %s", err.Error())
		return
	}
	t.Logf("formated %s", part)
	s, err := m.Statistics(os.TempDir())
	if err != nil {
		t.Errorf("GetStatistics failed with error: %s", err.Error())
		return
	}
	t.Logf("got %s statistics", os.TempDir())
	if s.AvailableBytes <= 0 {
		t.Errorf("GetStatistics failed available bytes if zero")
		return
	}

	if canMount() {
		if err := testFilesystemMountFilesystem(t, m, part); err != nil {
			t.Error(err)
			return
		}
		if err := testFilesystemMountBlockDevice(t, m, part); err != nil {
			t.Error(err)
			return
		}
	} else {
		t.Log("skipped mount testing")
	}
}

func testFilesystemMountFilesystem(t *testing.T, m *LinuxFilesystem, partition string) error {
	t.Helper()
	mountPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s-mount-path-%d", driverName, time.Now().Unix()))
	defer os.RemoveAll(mountPath)

	if err := m.Mount(context.Background(), partition, mountPath, "ext4"); err != nil {
		return fmt.Errorf("Mount failed with error: %w", err)
	}
	isMounted, err := m.IsMounted(context.Background(), mountPath)
	if err != nil {
		return fmt.Errorf("IsMounted failed with error: %w", err)
	}
	if !isMounted {
		return errors.New("IsMounted returned false")
	}

	t.Logf("mounted %s to %s", partition, mountPath)
	if err := m.Unmount(context.Background(), mountPath); err != nil {
		return fmt.Errorf("Unmount failed with error: %w", err)
	}
	t.Logf("unmounted %s", mountPath)
	return nil
}

func testFilesystemMountBlockDevice(t *testing.T, m *LinuxFilesystem, partition string) error {
	t.Helper()
	mountPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s-mount-path-%d", driverName, time.Now().Unix()))
	defer os.RemoveAll(mountPath)

	if err := m.Mount(context.Background(), partition, mountPath, "", "bind"); err != nil {
		return fmt.Errorf("Mount failed with error: %w", err)
	}
	isMounted, err := m.IsMounted(context.Background(), mountPath)
	if err != nil {
		return fmt.Errorf("IsMounted failed with error: %w", err)
	}
	if !isMounted {
		return errors.New("IsMounted returned false")
	}

	t.Logf("mounted %s to %s", partition, mountPath)
	if err := m.Unmount(context.Background(), mountPath); err != nil {
		return fmt.Errorf("Unmount failed with error: %w", err)
	}
	t.Logf("unmounted %s", mountPath)
	return nil
}

func TestFilesystemDisk(t *testing.T) {
	t.Parallel()
	if err := checkSystemRequirements(); err != nil {
		t.Skipf("skipping test: %s", err.Error())
	}
	// create 10MB fake disk
	disk, err := createDeviceFile(1e7)
	if err != nil {
		t.Error(err)
		return
	}
	defer os.Remove(disk)
	t.Logf("create fake disk device %s", disk)
	m := newTestFilesystem()

	// Create partition equivalent to creating /dev/sda1 to device /dev/sda
	if err := m.createPartition(context.Background(), disk); err != nil {
		t.Errorf("createPartition failed with error: %s", err.Error())
		return
	}

	// check if partition table exists
	gotFormated, err := m.isFormatted(context.Background(), disk)
	if err != nil {
		t.Errorf("IsFormatted failed with error: %s", err.Error())
		return
	}
	if gotFormated != true {
		t.Error("IsFormatted failed device should have parition table (GPT)")
		return
	}

	// check last partition
	wantPartition := disk + "p1"
	gotPartition, err := m.GetDeviceLastPartition(context.Background(), disk)
	if err != nil {
		t.Errorf("getLastPartition failed with error: %s", err.Error())
		return
	}
	if wantPartition != gotPartition {
		t.Errorf("getLastPartition failed want %s got %s", wantPartition, gotPartition)
		return
	}
	t.Logf("created new partition %s", wantPartition)
}

func createDeviceFile(size int64) (string, error) {
	f, err := os.CreateTemp(os.TempDir(), fmt.Sprintf("%s-disk-*", driverName))
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := f.Truncate(size); err != nil {
		return f.Name(), err
	}
	return f.Name(), err
}

func checkSystemRequirements() error {
	tools := []string{
		"mkfs.ext4", "mount", "umount", "blkid", "wipefs", "findmnt", "parted", "sfdisk", "tune2fs", "udevadm",
	}
	for _, t := range tools {
		if _, err := exec.LookPath(t); err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				return fmt.Errorf("%s executable not found in $PATH", t)
			}
			return err
		}
	}
	return nil
}

func newTestFilesystem() *LinuxFilesystem {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return NewLinuxFilesystem(logger.WithFields(nil))
}

func canMount() bool {
	return os.Getuid() == 0
}

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
	tempDir, err := os.MkdirTemp(os.TempDir(), fmt.Sprintf("test-%s-*", driverName))
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

func createTempFile(dir, pattern string) (string, error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	return f.Name(), f.Close()
}
