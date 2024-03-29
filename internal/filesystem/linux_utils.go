package filesystem

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	errNodeDiskNotFound  = errors.New("disk not found")
	errPartitionNotFound = errors.New("partition not found")
	ErrToolNotFound      = errors.New("tool not found")
)

// getBlockDeviceByDiskID returns actual block device path (e.g. /dev/vda) that correspond to disk ID (hardware serial number).
// diskID can be udev disk ID or path to disk ID symbolic link e.g. /dev/disk/by-id/virtio-014e425736724563ab83.
func getBlockDeviceByDiskID(ctx context.Context, diskID string) (dev string, err error) {
	ln := diskID
	if !filepath.IsAbs(diskID) {
		ln = filepath.Join(udevDiskByIDPath, diskID)
	}

	if err := udevWaitDiskToSettle(ctx, ln); err != nil {
		return ln, err
	}

	for s := time.Now(); time.Since(s) < time.Second*udevDiskTimeout; {
		dev, err = os.Readlink(ln)
		if err != nil && !os.IsNotExist(err) {
			return ln, err
		}
		if dev != "" {
			break
		}
		time.Sleep(time.Second * 2)
	}

	if dev == "" {
		return ln, errNodeDiskNotFound
	}

	if !filepath.IsAbs(dev) {
		dev, err = filepath.Abs(filepath.Join(filepath.Dir(ln), dev))
		if err != nil {
			return dev, err
		}
	}
	_, err = os.Stat(dev)
	return dev, err
}

// udevWaitDiskToSettle uses udevadm to wait events in event queue to be handled.
func udevWaitDiskToSettle(ctx context.Context, path string) error {
	if udevadm, err := exec.LookPath("udevadm"); err == nil {
		return exec.CommandContext(ctx,
			udevadm,
			"settle",
			fmt.Sprintf("--timeout=%d", udevSettleTimeout),
			fmt.Sprintf("--exit-if-exists=%s", path),
		).Run()
	}
	return nil
}

// volumeIDToDiskID converts volume ID to disk ID managed by udev e.g. f67db1ca-825b-40aa-a6f4-390ac6ff1b91 -> virtio-f67db1ca825b40aaa6f4.
func volumeIDToDiskID(volumeID string) (string, error) {
	fullID := strings.Join(strings.Split(volumeID, "-"), "")
	if len(fullID) <= 20 {
		return "", fmt.Errorf("volume ID '%s' too short", volumeID)
	}
	return diskPrefix + fullID[:20], nil
}

func sfdiskOutputGetLastPartition(source, sfdiskOutput string) (string, error) {
	outLines := strings.Split(sfdiskOutput, "\n")
	var lastPartition string
	for i := len(outLines) - 1; i >= 0; i-- {
		partition := strings.TrimSpace(outLines[i])
		if strings.HasPrefix(partition, source) {
			lastPartition = partition
			break
		}
	}
	if lastPartition == "" {
		return "", fmt.Errorf("unable to read last partition from sfdisk output [%s]; %w", strings.Join(outLines, ", "), errPartitionNotFound)
	}
	return lastPartition, nil
}

func createBlockDevice(target string) error {
	err := os.MkdirAll(filepath.Dir(target), 0o750)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_CREATE, 0o660)
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return nil
}

func checkToolsExists(tools ...string) error {
	for i := range tools {
		if _, err := exec.LookPath(tools[i]); err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				return fmt.Errorf("%q executable not found in $PATH; %w", tools[i], ErrToolNotFound)
			}
		}
	}
	return nil
}

func cmdExitCode(err error) int {
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode()
	}
	return 0
}

func formatCmdError(output []byte) string {
	return strings.Join(strings.Split(string(output), "\n"), " ")
}
