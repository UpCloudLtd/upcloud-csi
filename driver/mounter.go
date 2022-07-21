package driver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"k8s.io/mount-utils"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

const (
	// blkidExitStatusNoIdentifiers defines the exit code returned from blkid indicating that no devices have been found.
	blkidExitStatusNoIdentifiers = 2
	ext3                         = "ext3"
	ext4                         = "ext4"
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

type Mounter interface {
	// Format formats the source with the given filesystem type
	Format(source, fsType string, mkfsArgs []string) error

	// Mount mounts source to target with the given fstype and options.
	Mount(source, target, fsType string, options ...string) error

	// Unmount unmounts the given target
	Unmount(target string) error

	// IsFormatted checks whether the source device is formatted or not. It
	// returns true if the source device is already formatted.
	IsFormatted(source string) (bool, error)

	// IsMounted checks whether the target path is a correct mount (i.e:
	// propagated). It returns true if it's mounted. An error is returned in
	// case of system errors or if it's mounted incorrectly.
	IsMounted(target string) (bool, error)

	// IsPrepared checks whether a device is uniquely addressable
	isPrepared(target string) (string, error)

	// Sets the filesystem UUID to the same UUID as used within Upcloud for easy downstream handling
	setUUID(source, newUUID string) error

	// GetStatistics returns capacity-related volume statistics for the given
	// volume path.
	GetStatistics(volumePath string) (volumeStatistics, error)

	wipeDevice(deviceID string) error

	GetDeviceName(mounter mount.Interface, mountPath string) (string, error)
}

type mounter struct {
	log *logrus.Entry
}

// newMounter returns a new mounter instance.
func newMounter(log *logrus.Entry) *mounter {
	return &mounter{
		log: log,
	}
}

func (m *mounter) Format(source, fsType string, mkfsArgs []string) error {
	if fsType == "" {
		return errors.New("fs type is not specified for formatting the volume")
	}

	if source == "" {
		return errors.New("source is not specified for formatting the volume")
	}

	m.log.Infof("source: %s", source)

	m.log.Info("create partition called")
	err := createPartition(source)
	if err != nil {
		return err
	}

	lastPartition, err := getLastPartition(m.log)
	if err != nil {
		return err
	}

	if fsType == ext4 || fsType == ext3 {
		mkfsArgs = append(mkfsArgs, "-F", lastPartition)
	}

	mkfsCmd := fmt.Sprintf("mkfs.%s", fsType)

	_, err = exec.LookPath(mkfsCmd)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("%q executable not found in $PATH", mkfsCmd)
		}
		return err
	}

	m.log.WithFields(logrus.Fields{
		"cmd":  mkfsCmd,
		"args": mkfsArgs,
	}).Info("executing format command")

	out, err := exec.Command(mkfsCmd, mkfsArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("formatting disk failed: %w cmd: '%s %s' output: %q",
			err, mkfsCmd, strings.Join(mkfsArgs, " "), string(out))
	}

	return nil
}

func (m *mounter) Mount(source, target, fsType string, opts ...string) error {
	mountCmd := "mount"
	mountArgs := []string{}

	if fsType == "" {
		return errors.New("fs type is not specified for mounting the volume")
	}

	if source == "" {
		return errors.New("source is not specified for mounting the volume")
	}

	if target == "" {
		return errors.New("target is not specified for mounting the volume")
	}

	mountArgs = append(mountArgs, "-t", fsType)

	if len(opts) > 0 {
		mountArgs = append(mountArgs, "-o", strings.Join(opts, ","))
	}

	mountArgs = append(mountArgs, source, target)

	// create target, os.Mkdirall is noop if it exists
	err := os.MkdirAll(target, 0o750)
	if err != nil {
		return err
	}

	m.log.WithFields(logrus.Fields{
		"cmd":  mountCmd,
		"args": mountArgs,
	}).Info("executing mount command")

	out, err := exec.Command(mountCmd, mountArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("mounting failed: %w cmd: '%s %s' output: %q",
			err, mountCmd, strings.Join(mountArgs, " "), string(out))
	}

	return nil
}

func (m *mounter) Unmount(target string) error {
	umountCmd := "umount"
	if target == "" {
		return errors.New("target is not specified for unmounting the volume")
	}

	umountArgs := []string{target}

	m.log.WithFields(logrus.Fields{
		"cmd":  umountCmd,
		"args": umountArgs,
	}).Info("executing umount command")

	out, err := exec.Command(umountCmd, umountArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("unmounting failed: %w cmd: '%s %s' output: %q",
			err, umountCmd, target, string(out))
	}

	return nil
}

func (m *mounter) IsFormatted(source string) (bool, error) {
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

	m.log.WithFields(logrus.Fields{
		"cmd":  blkidCmd,
		"args": blkidArgs,
	}).Info("checking if source is formatted")

	exitCode := 0
	cmd := exec.Command(blkidCmd, blkidArgs...)
	err = cmd.Run()
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			return false, fmt.Errorf("checking formatting failed: %w cmd: %q, args: %q", err, blkidCmd, blkidArgs)
		}
		//nolint:errcheck // Error exit code is checked below
		ws := exitError.Sys().(syscall.WaitStatus)
		exitCode = ws.ExitStatus()
		if exitCode == blkidExitStatusNoIdentifiers {
			return false, nil
		}
		return false, fmt.Errorf("checking formatting failed: %w cmd: %q, args: %q", err, blkidCmd, blkidArgs)
	}

	return true, nil
}

func (m *mounter) isPrepared(source string) (string, error) {
	unformattedDevice := ""
	formatted, err := m.IsFormatted(source)
	if err != nil {
		return "", err
	}

	if !formatted {
		unformattedDevice = source
		return unformattedDevice, nil
	}
	return "", fmt.Errorf("no conclusive unformatted device found. recover manually")
}

func (m *mounter) wipeDevice(deviceID string) error {
	_, err := exec.Command("wipefs", "-a", deviceID).CombinedOutput()
	if err != nil {
		return fmt.Errorf("error wiping device %s", deviceID)
	}
	return nil
}

func (m *mounter) setUUID(source, newUUID string) error {
	findmntCmd := "tune2fs"
	findmntArgs := []string{"-U", newUUID, source}
	m.log.WithFields(logrus.Fields{
		"cmd":  findmntCmd,
		"args": findmntArgs,
	}).Info("setting uuid for new filesystem")

	cmd := exec.Command(findmntCmd, findmntArgs...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	defer stdin.Close()
	if err = cmd.Start(); err != nil {
		return err
	}

	if _, err = io.WriteString(stdin, "y\n"); err != nil {
		return err
	}

	if err = cmd.Wait(); err != nil {
		return err
	}

	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return exitError
		}
		return err
	}
	_, err = exec.Command("udevadm", "trigger").CombinedOutput()
	if err != nil {
		return fmt.Errorf("triggering udevadm failed - error: %w", err)
	}
	return nil
}

func (m *mounter) IsMounted(target string) (bool, error) {
	if target == "" {
		return false, errors.New("target is not specified for checking the mount")
	}

	findmntCmd := "findmnt"
	_, err := exec.LookPath(findmntCmd)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return false, fmt.Errorf("%q executable not found in $PATH", findmntCmd)
		}
		return false, err
	}

	findmntArgs := []string{"-o", "TARGET,PROPAGATION,FSTYPE,OPTIONS", "-M", target, "-J"}

	m.log.WithFields(logrus.Fields{
		"cmd":  findmntCmd,
		"args": findmntArgs,
	}).Info("checking if target is mounted")

	out, err := exec.Command(findmntCmd, findmntArgs...).CombinedOutput()
	if err != nil {
		// findmnt exits with non zero exit status if it couldn't find anything
		if strings.TrimSpace(string(out)) == "" {
			return false, nil
		}

		return false, fmt.Errorf("checking mounted failed: %w cmd: %q output: %q",
			err, findmntCmd, string(out))
	}

	// no response means there is no mount
	if len(out) == 0 {
		return false, nil
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

func (m *mounter) GetStatistics(volumePath string) (volumeStatistics, error) {
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

func (m *mounter) GetDeviceName(mounter mount.Interface, mountPath string) (string, error) {
	devicePath, _, err := mount.GetDeviceNameFromMount(mounter, mountPath)
	return devicePath, err
}

func getLastPartition(log *logrus.Entry) (string, error) {
	log.Info("get last partition called")
	sfdisk := exec.Command("sfdisk", "-q", "--list")
	awk := exec.Command("awk", "NR>1{print $1}")

	r, w := io.Pipe()
	sfdisk.Stdout = w
	awk.Stdin = r

	var buf bytes.Buffer
	awk.Stdout = &buf

	if err := sfdisk.Start(); err != nil {
		return "", err
	}
	if err := awk.Start(); err != nil {
		return "", err
	}
	if err := sfdisk.Wait(); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	if err := awk.Wait(); err != nil {
		return "", err
	}

	out := buf.String()
	partitions := strings.Split(out, "\n")

	var lastPartition string
	for i := len(partitions) - 1; i > 0; i-- {
		if !strings.HasPrefix(partitions[i], "/dev") {
			continue
		} else {
			lastPartition = partitions[i]
			break
		}
	}

	return lastPartition, nil
}

func createPartition(device string) error {
	partedMklabelOut, err := exec.Command("parted", device, "mklabel", "gpt").CombinedOutput()
	if err != nil {
		return err
	}
	logrus.Printf("parted mklabel output: %s\n", partedMklabelOut)

	partedCreatePartitionOut, err := exec.Command("parted", "-a", "opt", device, "mkpart", "primary", "2048s", "100%").CombinedOutput()
	if err != nil {
		return err
	}
	logrus.Printf("mkpart output: %s\n", partedCreatePartitionOut)

	return nil
}
