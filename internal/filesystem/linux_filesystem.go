package filesystem

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/UpCloudLtd/upcloud-csi/internal/logger"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

const (
	udevDiskByIDPath  = "/dev/disk/by-id"
	diskPrefix        = "virtio-"
	maxVolumesPerNode = 14
	fileSystemExt4    = "ext4"

	// udevDiskTimeout specifies a time limit for waiting disk appear under /dev/disk/by-id.
	udevDiskTimeout = 60
	// udevSettleTimeout specifies a time limit for waiting udev event queue to become empty.
	udevSettleTimeout = 20
)

type LinuxFilesystem struct {
	log *logrus.Entry
}

func NewLinuxFilesystem(log *logrus.Entry) *LinuxFilesystem {
	return &LinuxFilesystem{
		log: log,
	}
}

// Format formats the source with the given filesystem type.
func (m *LinuxFilesystem) Format(ctx context.Context, source, fsType string, mkfsArgs []string) error {
	if fsType == "" {
		return errors.New("fs type is not specified for formatting the volume")
	}

	if source == "" {
		return errors.New("source is not specified for formatting the volume")
	}

	formatted, err := m.isFormatted(ctx, source)
	if err != nil {
		return err
	}
	if formatted {
		return nil
	}

	err = m.createPartition(ctx, source)
	if err != nil {
		return err
	}

	lastPartition, err := m.GetDeviceLastPartition(ctx, source)
	if err != nil {
		return err
	}
	return m.createFilesystem(ctx, lastPartition, fsType, mkfsArgs)
}

func (m *LinuxFilesystem) createFilesystem(ctx context.Context, partition, fsType string, mkfsArgs []string) error {
	if fsType == fileSystemExt4 || fsType == "ext3" {
		mkfsArgs = append(mkfsArgs, "-F", partition)
	}

	mkfsCmd := fmt.Sprintf("mkfs.%s", fsType)

	_, err := exec.LookPath(mkfsCmd)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("%q executable not found in $PATH", mkfsCmd)
		}
		return err
	}

	logger.WithServerContext(ctx, m.log).WithFields(logrus.Fields{logger.CommandKey: mkfsCmd, logger.CommandArgsKey: mkfsArgs}).Debug("executing command")

	return exec.CommandContext(ctx, mkfsCmd, mkfsArgs...).Run()
}

// Mount mounts source to target with the given fstype and options.
func (m *LinuxFilesystem) Mount(ctx context.Context, source, target, fsType string, opts ...string) error {
	mountCmd := "mount"
	mountArgs := make([]string, 0)

	if source == "" {
		return errors.New("source is not specified for mounting the volume")
	}

	if target == "" {
		return errors.New("target is not specified for mounting the volume")
	}

	// block device requires that target is file instead of directory
	if fsType == "" {
		err := createBlockDevice(target)
		if err != nil {
			return err
		}
	} else {
		mountArgs = append(mountArgs, "-t", fsType)
		// create target, os.Mkdirall is noop if it exists
		err := os.MkdirAll(target, 0o750)
		if err != nil {
			return err
		}
	}

	if len(opts) > 0 {
		mountArgs = append(mountArgs, "-o", strings.Join(opts, ","))
	}

	mountArgs = append(mountArgs, source, target)

	logger.WithServerContext(ctx, m.log).WithFields(logrus.Fields{logger.CommandKey: mountCmd, logger.CommandArgsKey: mountArgs}).Debug("executing command")

	return exec.CommandContext(ctx, mountCmd, mountArgs...).Run()
}

// Unmount unmounts the given target.
func (m *LinuxFilesystem) Unmount(ctx context.Context, target string) error {
	log := logger.WithServerContext(ctx, m.log)
	if target == "" {
		return errors.New("target is not specified for unmounting the volume")
	}

	if _, err := os.Stat(target); os.IsNotExist(err) {
		log.WithFields(logrus.Fields{"target": target}).Debug("unmount target does not exist")
		return nil
	}

	umountCmd := "umount"
	umountArgs := []string{target}

	logger.WithServerContext(ctx, m.log).WithFields(logrus.Fields{logger.CommandKey: umountCmd, logger.CommandArgsKey: umountArgs}).Debug("executing command")

	return exec.CommandContext(ctx, umountCmd, umountArgs...).Run()
}

// IsFormatted checks whether the source device is formatted or not. It
// returns true if the source device is already formatted.
func (m *LinuxFilesystem) isFormatted(ctx context.Context, source string) (bool, error) {
	// blkidExitStatusNoIdentifiers defines the exit code returned from blkid indicating that no devices have been found.
	// See http://www.polarhome.com/service/man/?qf=blkid&tf=2&of=Alpinelinux for details.
	const blkidExitStatusNoIdentifiers = 2

	if source == "" {
		return false, errors.New("source is not specified")
	}

	blkidCmd := "blkid"
	_, err := exec.LookPath(blkidCmd)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return false, fmt.Errorf("%q executable not found in $PATH", blkidCmd)
		}
		return false, err
	}

	blkidArgs := []string{source}

	logger.WithServerContext(ctx, m.log).WithFields(logrus.Fields{logger.CommandKey: blkidCmd, logger.CommandArgsKey: blkidArgs}).Debug("executing command")
	if err = exec.CommandContext(ctx, blkidCmd, blkidArgs...).Run(); err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			return false, fmt.Errorf("checking formatting failed: %w cmd: %q, args: %q", err, blkidCmd, blkidArgs)
		}
		ws, ok := exitError.Sys().(syscall.WaitStatus)
		if !ok {
			return false, fmt.Errorf("checking formatting exit status: %w cmd: %q, args: %q", err, blkidCmd, blkidArgs)
		}
		exitCode := ws.ExitStatus()
		if exitCode == blkidExitStatusNoIdentifiers {
			return false, nil
		}
		return false, fmt.Errorf("checking formatting failed: %w cmd: %q, args: %q", err, blkidCmd, blkidArgs)
	}

	return true, nil
}

// IsMounted checks whether the target path is a correct mount (i.e:
// propagated). It returns true if it's mounted. An error is returned in
// case of system errors or if it's mounted incorrectly.
func (m *LinuxFilesystem) IsMounted(ctx context.Context, target string) (bool, error) {
	if target == "" {
		return false, errors.New("target is not specified for checking the mount")
	}

	findmntCmd := "findmnt"
	findmntArgs := []string{"-o", "TARGET,PROPAGATION,FSTYPE,OPTIONS", "-M", target, "-J"}

	logger.WithServerContext(ctx, m.log).WithFields(logrus.Fields{logger.CommandKey: findmntCmd, logger.CommandArgsKey: findmntArgs}).Debug("executing command")

	out, err := exec.CommandContext(ctx, findmntCmd, findmntArgs...).CombinedOutput()
	if err != nil {
		// findmnt exits with non zero exit status if it couldn't find anything
		if strings.TrimSpace(string(out)) == "" {
			return false, nil
		}

		return false, fmt.Errorf("checking mounted failed: %w cmd: %q output: %q", err, findmntCmd, string(out))
	}

	// no response means there is no mount
	if len(out) == 0 {
		return false, nil
	}

	type fileSystem struct {
		Target      string `json:"target"`
		Propagation string `json:"propagation"`
		FsType      string `json:"fstype"`
		Options     string `json:"options"`
	}

	type findmntResponse struct {
		FileSystems []fileSystem `json:"filesystems"`
	}

	var resp *findmntResponse
	err = json.Unmarshal(out, &resp)
	if err != nil {
		return false, fmt.Errorf("couldn't unmarshal data: %q: %w", string(out), err)
	}

	targetFound := false
	for _, fs := range resp.FileSystems {
		// check if the mount is propagated correctly. It should be set to shared.
		if fs.Propagation != "shared" {
			return true, fmt.Errorf("mount propagation for target %q is not enabled", target)
		}

		// the mountpoint should match as well
		if fs.Target == target {
			targetFound = true
		}
	}

	return targetFound, nil
}

func (m *LinuxFilesystem) createPartition(ctx context.Context, device string) error {
	cmd := "parted"
	args := []string{device, "mklabel", "gpt"}
	log := logger.WithServerContext(ctx, m.log).WithFields(logrus.Fields{logger.CommandKey: cmd, logger.CommandArgsKey: args})
	log.Debug("executing command")
	partedMklabel := exec.CommandContext(ctx, cmd, args...)
	if err := partedMklabel.Run(); err != nil {
		return err
	}

	args = []string{"-a", "opt", device, "mkpart", "primary", "2048s", "100%"}
	logger.WithServerContext(ctx, m.log).WithFields(logrus.Fields{logger.CommandKey: cmd, logger.CommandArgsKey: args}).Debug("executing command")
	return exec.CommandContext(ctx, cmd, args...).Run()
}

// filesystemStatistics returns capacity-related volume statistics for the given volume path.
func (m *LinuxFilesystem) Statistics(volumePath string) (VolumeStatistics, error) {
	var statfs unix.Statfs_t
	// See http://man7.org/linux/man-pages/man2/statfs.2.html for details.
	err := unix.Statfs(volumePath, &statfs)
	if err != nil {
		return VolumeStatistics{}, err
	}
	volStats := VolumeStatistics{
		AvailableBytes: int64(statfs.Bavail) * int64(statfs.Bsize),                         //nolint:unconvert // unix.Statfs_t integer types varies between GOARCHs
		TotalBytes:     int64(statfs.Blocks) * int64(statfs.Bsize),                         //nolint:unconvert // unix.Statfs_t integer types varies between GOARCHs
		UsedBytes:      (int64(statfs.Blocks) - int64(statfs.Bfree)) * int64(statfs.Bsize), //nolint:unconvert // unix.Statfs_t integer types varies between GOARCHs

		AvailableInodes: int64(statfs.Ffree),
		TotalInodes:     int64(statfs.Files),
		UsedInodes:      int64(statfs.Files) - int64(statfs.Ffree),
	}

	return volStats, nil
}

// getBlockDeviceByVolumeID returns the absolute path of the attached block device for the given volumeID.
func (m *LinuxFilesystem) GetDeviceByID(ctx context.Context, id string) (string, error) {
	diskID, err := volumeIDToDiskID(id)
	if err != nil {
		return diskID, err
	}
	return getBlockDeviceByDiskID(ctx, diskID)
}

func (m *LinuxFilesystem) GetDeviceLastPartition(ctx context.Context, source string) (string, error) {
	sfdisk, err := exec.CommandContext(ctx, "sfdisk", "-q", "--list", "-o", "device", source).CombinedOutput()
	if err != nil {
		return "", err
	}
	return sfdiskOutputGetLastPartition(source, string(sfdisk))
}
