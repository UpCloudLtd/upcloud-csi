package filesystem

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/UpCloudLtd/upcloud-csi/internal/logger"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

const (
	udevDiskByIDPath        = "/dev/disk/by-id"
	diskPrefix              = "virtio-"
	partitionTableType      = "gpt"
	blkidCmd                = "blkid"
	blkidCmdErrCodeNotFound = 2
	partedCmd               = "parted"
	sfdiskCmd               = "sfdisk"
	// udevDiskTimeout specifies a time limit for waiting disk appear under /dev/disk/by-id.
	udevDiskTimeout = 60
	// udevSettleTimeout specifies a time limit for waiting udev event queue to become empty.
	udevSettleTimeout = 20
)

type LinuxFilesystem struct {
	log             *logrus.Entry
	filesystemTypes []string
}

func NewLinuxFilesystem(filesystemTypes []string, log *logrus.Entry) (*LinuxFilesystem, error) {
	tools := []string{blkidCmd, partedCmd, sfdiskCmd}
	for i := range filesystemTypes {
		tools = append(tools, fmt.Sprintf("mkfs.%s", filesystemTypes[i]))
	}

	return &LinuxFilesystem{
		log:             log,
		filesystemTypes: filesystemTypes,
	}, checkToolsExists(tools...) // allow caller to decide what to do if tools are not present
}

// Format writes new partition table and creates new partition and filesystem.
// This function should be idempotent and should eventually lead to success
// in case off temporary system failure.
func (m *LinuxFilesystem) Format(ctx context.Context, source, fsType string, mkfsArgs []string) error {
	if fsType == "" {
		return errors.New("fs type is not specified for formatting the volume")
	}
	if source == "" {
		return errors.New("source is not specified for formatting the volume")
	}

	fsType = strings.ToLower(fsType)
	if err := m.isSupportedFilesystem(fsType); err != nil {
		return err
	}
	if err := m.createPartitionTableIfNotExists(ctx, source); err != nil {
		return err
	}
	partition, err := m.createPartitionIfNotExists(ctx, source)
	if err != nil {
		return err
	}
	return m.createFilesystemIfNotExists(ctx, partition, fsType, mkfsArgs)
}

func (m *LinuxFilesystem) isSupportedFilesystem(fsType string) error {
	for i := range m.filesystemTypes {
		if strings.Compare(m.filesystemTypes[i], fsType) == 0 {
			return nil
		}
	}
	return fmt.Errorf("filesystem type '%s' is not supported", fsType)
}

// createFilesystem creates new filesystem if one doesn't exists yet.
func (m *LinuxFilesystem) createFilesystemIfNotExists(ctx context.Context, partition, fsType string, mkfsArgs []string) error {
	if ok, err := m.filesystemExists(ctx, partition, fsType); ok || err != nil {
		return err
	}
	mkfsArgs = append(mkfsArgs, partition)
	mkfsCmd := fmt.Sprintf("mkfs.%s", fsType)

	logger.WithServerContext(ctx, m.log).WithFields(logrus.Fields{logger.CommandKey: mkfsCmd, logger.CommandArgsKey: mkfsArgs}).Debug("executing command")
	output, err := exec.CommandContext(ctx, mkfsCmd, mkfsArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create filesystem %s %s (%s); %w", mkfsCmd, strings.Join(mkfsArgs, " "), formatCmdError(output), err)
	}
	return nil
}

// filesystemExists checks whether the partition is formatted or not. It
// returns true if the source device is already formatted.
func (m *LinuxFilesystem) filesystemExists(ctx context.Context, partition, fsType string) (bool, error) {
	if partition == "" {
		return false, errors.New("partition is not specified")
	}
	blkidArgs := []string{
		// low-level superblocks probing (bypass cache)
		"--probe",
		// output format
		"--output", "value",
		// show specified tag
		"--match-tag", "TYPE",
		// find device with a specific token (NAME=value pair)
		"--match-token", fmt.Sprintf("TYPE=%s", fsType),
		partition,
	}

	logger.WithServerContext(ctx, m.log).WithFields(logrus.Fields{logger.CommandKey: blkidCmd, logger.CommandArgsKey: blkidArgs}).Debug("executing command")
	output, err := exec.CommandContext(ctx, blkidCmd, blkidArgs...).CombinedOutput()
	if err != nil {
		if cmdExitCode(err) == blkidCmdErrCodeNotFound {
			return false, nil
		}
		return false, fmt.Errorf("checking partition filesystem failed: %w (%s)", err, formatCmdError(output))
	}
	if strings.TrimSpace(strings.ToLower(string(output))) == fsType {
		return true, nil
	}
	return false, nil
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
		return false, fmt.Errorf("checking mounted failed: %w cmd: %q output: %s", err, findmntCmd, formatCmdError(out))
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

// createPartitionTableIfNotExists creates new partition table if one does not exists.
func (m *LinuxFilesystem) createPartitionTableIfNotExists(ctx context.Context, device string) error {
	if ok, err := m.hasPartitionTable(ctx, device); ok || err != nil {
		return err
	}
	args := []string{device, "mklabel", "gpt"}
	log := logger.WithServerContext(ctx, m.log).WithFields(logrus.Fields{logger.CommandKey: partedCmd, logger.CommandArgsKey: args})
	log.Debug("executing command")
	output, err := exec.CommandContext(ctx, partedCmd, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create %s partition table '%s'; %w", device, formatCmdError(output), err)
	}
	return nil
}

func (m *LinuxFilesystem) hasPartitionTable(ctx context.Context, device string) (bool, error) {
	if device == "" {
		return false, errors.New("source is not specified")
	}
	blkidArgs := []string{
		// low-level superblocks probing (bypass cache)
		"--probe",
		// output format
		"--output", "value",
		// show specified tag
		"--match-tag", "PTTYPE",
		// find device with a specific token (NAME=value pair)
		"--match-token", fmt.Sprintf("PTTYPE=%s", partitionTableType),
		device,
	}

	log := logger.WithServerContext(ctx, m.log)
	log.WithFields(logrus.Fields{logger.CommandKey: blkidCmd, logger.CommandArgsKey: blkidArgs}).Debug("executing command")
	output, err := exec.CommandContext(ctx, blkidCmd, blkidArgs...).CombinedOutput()
	if err != nil {
		if cmdExitCode(err) == blkidCmdErrCodeNotFound {
			return false, nil
		}
		return false, fmt.Errorf("checking %s partition table failed: %w (%s)", device, err, formatCmdError(output))
	}
	foundType := strings.TrimSpace(strings.ToLower(string(output)))
	log.WithField("device", device).Infof("existing partition table %s found", foundType)
	if foundType == partitionTableType {
		return true, nil
	}
	return false, nil
}

// createPartitionIfNotExists creates new primary partition if one does not exists.
func (m *LinuxFilesystem) createPartitionIfNotExists(ctx context.Context, device string) (string, error) {
	log := logger.WithServerContext(ctx, m.log)
	if p, err := m.GetDeviceLastPartition(ctx, device); err == nil {
		log.WithField("partition", p).Info("existing partition found")
		return p, nil
	}
	args := []string{"-a", "opt", device, "mkpart", "primary", "2048s", "100%"}
	log.WithFields(logrus.Fields{logger.CommandKey: partedCmd, logger.CommandArgsKey: args}).Debug("executing command")
	output, err := exec.CommandContext(ctx, partedCmd, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create new partition: '%s'; %w", formatCmdError(output), err)
	}
	return m.GetDeviceLastPartition(ctx, device)
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

func (m *LinuxFilesystem) GetDeviceLastPartition(ctx context.Context, device string) (string, error) {
	output, err := exec.CommandContext(ctx, sfdiskCmd, "-q", "--list", "-o", "device", device).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get %s last partition: '%s'; %w", device, formatCmdError(output), err)
	}

	return sfdiskOutputGetLastPartition(device, string(output))
}
