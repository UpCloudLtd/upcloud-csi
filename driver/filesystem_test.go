package driver //nolint:testpackage // use conventional naming for now

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

	"github.com/sirupsen/logrus"
)

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

func testFilesystemMountFilesystem(t *testing.T, m *nodeFilesystem, partition string) error {
	t.Helper()
	mountPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s-mount-path-%d", DefaultDriverName, time.Now().Unix()))
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

func testFilesystemMountBlockDevice(t *testing.T, m *nodeFilesystem, partition string) error {
	t.Helper()
	mountPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s-mount-path-%d", DefaultDriverName, time.Now().Unix()))
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
	f, err := os.CreateTemp(os.TempDir(), fmt.Sprintf("%s-disk-*", DefaultDriverName))
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

func newTestFilesystem() *nodeFilesystem {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return newNodeFilesystem(logger.WithFields(nil))
}

func canMount() bool {
	return os.Getuid() == 0
}
