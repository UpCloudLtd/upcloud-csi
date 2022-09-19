package driver

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestSfdiskOutputGetLastPartition(t *testing.T) {
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

func TestMounterFilesystem(t *testing.T) {
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

	m := newTestMounter()

	if err := m.format(part, "ext4", nil); err != nil {
		t.Errorf("Format failed with error: %s", err.Error())
		return
	}
	t.Logf("formated %s", part)
	s, err := m.GetStatistics(os.TempDir())
	if err != nil {
		t.Errorf("GetStatistics failed with error: %s", err.Error())
		return
	}
	t.Logf("got %s statistics", os.TempDir())
	if s.availableBytes <= 0 {
		t.Errorf("GetStatistics failed available bytes if zero")
		return
	}

	if canMount() {
		mountPath, err := os.MkdirTemp(os.TempDir(), fmt.Sprintf("%s-mount-path-*", DefaultDriverName))
		if err != nil {
			t.Error(err)
			return
		}
		defer os.RemoveAll(mountPath)

		if err := m.Mount(part, mountPath, "ext4"); err != nil {
			t.Errorf("Mount failed with error: %s", err.Error())
			return
		}
		isMounted, err := m.IsMounted(mountPath)
		if err != nil {
			t.Errorf("IsMounted failed with error: %s", err.Error())
			return
		}
		if !isMounted {
			t.Errorf("IsMounted returned false")
			return
		}

		t.Logf("mounted %s to %s", part, mountPath)
		if err := m.Unmount(mountPath); err != nil {
			t.Errorf("Unmount failed with error: %s", err.Error())
			return
		}
		t.Logf("unmounted %s", mountPath)
	} else {
		t.Log("skipped mount testing")
	}
}

func TestMounterDisk(t *testing.T) {
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
	m := newTestMounter()

	// Create partition equivalent to creating /dev/sda1 to device /dev/sda
	if err := m.createPartition(disk); err != nil {
		t.Errorf("createPartition failed with error: %s", err.Error())
		return
	}

	// check if partition table exists
	gotFormated, err := m.IsFormatted(disk)
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
	gotPartition, err := getLastPartition(disk)
	if err != nil {
		t.Errorf("getLastPartition failed with error: %s", err.Error())
		return
	}
	if wantPartition != gotPartition {
		t.Errorf("getLastPartition failed want %s got %s", wantPartition, gotPartition)
		return
	}
	t.Logf("created new partition %s", wantPartition)

	// Wipe signatures from a device.
	if err := m.wipeDevice(disk); err != nil {
		t.Errorf("wipeDevice failed with error: %s", err.Error())
		return
	}
	t.Logf("wiped signatures from a device %s", disk)
	wantPartition = ""
	gotPartition, err = getLastPartition(disk)
	if err != nil {
		t.Errorf("getLastPartition after wipe failed with error: %s", err.Error())
		return
	}
	if wantPartition != gotPartition {
		t.Errorf("getLastPartition failed want %s got %s", wantPartition, gotPartition)
		return
	}
	t.Logf("device %s is now empty", disk)
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
			if err == exec.ErrNotFound {
				return fmt.Errorf("%s executable not found in $PATH", t)
			}
			return err
		}
	}
	return nil
}

func newTestMounter() *mounter {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return newMounter(logger.WithFields(nil))
}

func canMount() bool {
	return os.Getuid() == 0
}
