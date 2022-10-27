package driver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"k8s.io/mount-utils"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

type findmntResponse struct {
	FileSystems []fileSystem `json:"filesystems"`
}

type fileSystem struct {
	Target      string `json:"target"`
	Propagation string `json:"propagation"`
	FsType      string `json:"fstype"`
	Options     string `json:"options"`
}

type volumeStatistics struct {
	availableBytes, totalBytes, usedBytes    int64
	availableInodes, totalInodes, usedInodes int64
}

const (
	// blkidExitStatusNoIdentifiers defines the exit code returned from blkid indicating that no devices have been found. See http://www.polarhome.com/service/man/?qf=blkid&tf=2&of=Alpinelinux for details.
	blkidExitStatusNoIdentifiers = 2
)

type mounter struct {
	log *logrus.Entry
}

// newMounter returns a new mounter instance
func newMounter(log *logrus.Entry) *mounter {
	return &mounter{
		log: log,
	}
}

// Format formats the source with the given filesystem type
func (m *mounter) Format(ctx context.Context, source, fsType string, mkfsArgs []string) error {
	if fsType == "" {
		return errors.New("fs type is not specified for formatting the volume")
	}

	if source == "" {
		return errors.New("source is not specified for formatting the volume")
	}

	err := m.createPartition(ctx, source)
	if err != nil {
		return err
	}

	lastPartition, err := getLastPartition(ctx, source)
	if err != nil {
		return err
	}
	return m.createFilesystem(ctx, lastPartition, fsType, mkfsArgs)
}

func (m *mounter) createFilesystem(ctx context.Context, partition, fsType string, mkfsArgs []string) error {
	if fsType == "ext4" || fsType == "ext3" {
		mkfsArgs = append(mkfsArgs, "-F", partition)
	}

	mkfsCmd := fmt.Sprintf("mkfs.%s", fsType)

	_, err := exec.LookPath(mkfsCmd)
	if err != nil {
		if err == exec.ErrNotFound {
			return fmt.Errorf("%q executable not found in $PATH", mkfsCmd)
		}
		return err
	}

	logWithServerContext(m.log, ctx).WithFields(logrus.Fields{logCommandKey: mkfsCmd, logCommandArgsKey: mkfsArgs}).Debug("executing command")

	return exec.CommandContext(ctx, mkfsCmd, mkfsArgs...).Run()
}

// Mount mounts source to target with the given fstype and options.
func (m *mounter) Mount(ctx context.Context, source, target, fsType string, opts ...string) error {
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
		err := os.MkdirAll(filepath.Dir(target), 0750)
		if err != nil {
			return err
		}
		f, err := os.OpenFile(target, os.O_CREATE, 0660)
		if err != nil {
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	} else {
		mountArgs = append(mountArgs, "-t", fsType)
		// create target, os.Mkdirall is noop if it exists
		err := os.MkdirAll(target, 0750)
		if err != nil {
			return err
		}
	}

	if len(opts) > 0 {
		mountArgs = append(mountArgs, "-o", strings.Join(opts, ","))
	}

	mountArgs = append(mountArgs, source, target)

	logWithServerContext(m.log, ctx).WithFields(logrus.Fields{logCommandKey: mountCmd, logCommandArgsKey: mountArgs}).Debug("executing command")

	return exec.CommandContext(ctx, mountCmd, mountArgs...).Run()
}

// Unmount unmounts the given target
func (m *mounter) Unmount(ctx context.Context, target string) error {
	log := logWithServerContext(m.log, ctx)
	if target == "" {
		return errors.New("target is not specified for unmounting the volume")
	}

	if _, err := os.Stat(target); os.IsNotExist(err) {
		log.WithFields(logrus.Fields{"target": target}).Debug("unmount target does not exist")
		return nil
	}

	umountCmd := "umount"
	umountArgs := []string{target}

	logWithServerContext(m.log, ctx).WithFields(logrus.Fields{logCommandKey: umountCmd, logCommandArgsKey: umountArgs}).Debug("executing command")

	return exec.CommandContext(ctx, umountCmd, umountArgs...).Run()
}

// IsFormatted checks whether the source device is formatted or not. It
// returns true if the source device is already formatted.
func (m *mounter) IsFormatted(ctx context.Context, source string) (bool, error) {
	if source == "" {
		return false, errors.New("source is not specified")
	}

	blkidCmd := "blkid"
	_, err := exec.LookPath(blkidCmd)
	if err != nil {
		if err == exec.ErrNotFound {
			return false, fmt.Errorf("%q executable not found in $PATH", blkidCmd)
		}
		return false, err
	}

	blkidArgs := []string{source}

	logWithServerContext(m.log, ctx).WithFields(logrus.Fields{logCommandKey: blkidCmd, logCommandArgsKey: blkidArgs}).Debug("executing command")
	exitCode := 0
	if err = exec.CommandContext(ctx, blkidCmd, blkidArgs...).Run(); err != nil {
		exitError, ok := err.(*exec.ExitError)
		if !ok {
			return false, fmt.Errorf("checking formatting failed: %v cmd: %q, args: %q", err, blkidCmd, blkidArgs)
		}
		ws := exitError.Sys().(syscall.WaitStatus)
		exitCode = ws.ExitStatus()
		if exitCode == blkidExitStatusNoIdentifiers {
			return false, nil
		} else {
			return false, fmt.Errorf("checking formatting failed: %v cmd: %q, args: %q", err, blkidCmd, blkidArgs)
		}
	}

	return true, nil
}

func (m *mounter) wipeDevice(ctx context.Context, deviceId string) error {
	cmd := "wipefs"
	args := []string{"-a", "-f", deviceId}

	logWithServerContext(m.log, ctx).WithFields(logrus.Fields{logCommandKey: cmd, logCommandArgsKey: args}).Debug("executing command")

	return exec.CommandContext(ctx, cmd, args...).Run()
}

// IsMounted checks whether the target path is a correct mount (i.e:
// propagated). It returns true if it's mounted. An error is returned in
// case of system errors or if it's mounted incorrectly.
func (m *mounter) IsMounted(ctx context.Context, target string) (bool, error) {
	if target == "" {
		return false, errors.New("target is not specified for checking the mount")
	}

	findmntCmd := "findmnt"
	_, err := exec.LookPath(findmntCmd)
	if err != nil {
		if err == exec.ErrNotFound {
			return false, fmt.Errorf("%q executable not found in $PATH", findmntCmd)
		}
		return false, err
	}

	findmntArgs := []string{"-o", "TARGET,PROPAGATION,FSTYPE,OPTIONS", "-M", target, "-J"}

	logWithServerContext(m.log, ctx).WithFields(logrus.Fields{logCommandKey: findmntCmd, logCommandArgsKey: findmntArgs}).Debug("executing command")

	out, err := exec.CommandContext(ctx, findmntCmd, findmntArgs...).CombinedOutput()
	if err != nil {
		// findmnt exits with non zero exit status if it couldn't find anything
		if strings.TrimSpace(string(out)) == "" {
			return false, nil
		}

		return false, fmt.Errorf("checking mounted failed: %v cmd: %q output: %q",
			err, findmntCmd, string(out))
	}

	// no response means there is no mount
	if string(out) == "" {
		return false, nil
	}

	var resp *findmntResponse
	err = json.Unmarshal(out, &resp)
	if err != nil {
		return false, fmt.Errorf("couldn't unmarshal data: %q: %s", string(out), err)
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

// GetStatistics returns capacity-related volume statistics for the given volume path.
func (m *mounter) GetStatistics(ctx context.Context, volumePath string) (volumeStatistics, error) {
	var statfs unix.Statfs_t
	// See http://man7.org/linux/man-pages/man2/statfs.2.html for details.
	err := unix.Statfs(volumePath, &statfs)
	if err != nil {
		return volumeStatistics{}, err
	}
	volStats := volumeStatistics{
		availableBytes: int64(statfs.Bavail) * int64(statfs.Bsize),
		totalBytes:     int64(statfs.Blocks) * int64(statfs.Bsize),
		usedBytes:      (int64(statfs.Blocks) - int64(statfs.Bfree)) * int64(statfs.Bsize),

		availableInodes: int64(statfs.Ffree),
		totalInodes:     int64(statfs.Files),
		usedInodes:      int64(statfs.Files) - int64(statfs.Ffree),
	}

	return volStats, nil
}

func (m *mounter) GetDeviceName(ctx context.Context, mounter mount.Interface, mountPath string) (string, error) {
	devicePath, _, err := mount.GetDeviceNameFromMount(mounter, mountPath)
	return devicePath, err
}

func (m *mounter) createPartition(ctx context.Context, device string) error {

	cmd := "parted"
	args := []string{device, "mklabel", "gpt"}
	log := logWithServerContext(m.log, ctx).WithFields(logrus.Fields{logCommandKey: cmd, logCommandArgsKey: args})
	log.Debug("executing command")
	partedMklabel := exec.CommandContext(ctx, cmd, args...)
	if err := partedMklabel.Run(); err != nil {
		return err
	}

	args = []string{"-a", "opt", device, "mkpart", "primary", "2048s", "100%"}
	logWithServerContext(m.log, ctx).WithFields(logrus.Fields{logCommandKey: cmd, logCommandArgsKey: args}).Debug("executing command")
	return exec.CommandContext(ctx, cmd, args...).Run()
}

func getLastPartition(ctx context.Context, source string) (string, error) {
	sfdisk, err := exec.CommandContext(ctx, "sfdisk", "-q", "--list", "-o", "device", source).CombinedOutput()
	if err != nil {
		return "", err
	}
	return sfdiskOutputGetLastPartition(source, string(sfdisk))
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
	return lastPartition, nil
}
