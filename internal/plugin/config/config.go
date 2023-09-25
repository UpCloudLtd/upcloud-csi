package config

import (
	"errors"
	"os"
	"strings"

	"github.com/UpCloudLtd/upcloud-csi/internal/filesystem"
	"github.com/spf13/pflag"
)

const (

	// DefaultDriverName defines the name that is used in Kubernetes and the CSI
	// system for the canonical, official name of this plugin.
	DefaultDriverName string = "storage.csi.upcloud.com"
	// DefaultAddress is the default address that the csi plugin will serve its
	// http handler on.
	DefaultHealtServerAddress string = "tcp://127.0.0.1:13071"
	// DefaultPluginServerAddress is the default endpoint that the csi plugin will serve its
	// GRPC handlers on.
	DefaultPluginServerAddress string = "unix:///var/lib/kubelet/plugins/" + DefaultDriverName + "/csi.sock"

	// MaxVolumesPerNode is maxium volume count that one node can handle.
	MaxVolumesPerNode int = 14

	DriverModeMonolith   string = "monolith"
	DriverModeNode       string = "node"
	DriverModeController string = "controller"
	DefaultDriverMode    string = DriverModeMonolith

	envUpcloudUsername string = "UPCLOUD_USERNAME"
	envUpcloudPassword string = "UPCLOUD_PASSWORD"
	envStorageLabels   string = "STORAGE_LABELS"
)

type Config struct {
	NodeHost        string
	Zone            string
	Username        string
	Password        string
	DriverName      string
	PrintVersion    bool
	Mode            string
	LogLevel        string
	Labels          []string
	FilesystemTypes []string

	PluginServerAddress string
	HealtServerAddress  string

	Filesystem filesystem.Filesystem
}

func Parse(osArgs []string) (Config, error) {
	flagSet := pflag.NewFlagSet("default", pflag.ContinueOnError)
	c := Config{}
	flagSet.StringVar(&c.PluginServerAddress, "endpoint", DefaultPluginServerAddress, "CSI endpoint")
	flagSet.StringVar(&c.NodeHost, "nodehost", "", "Node's hostname. This should match server's `hostname` in the hub.upcloud.com.")
	flagSet.StringVar(&c.Zone, "zone", "", "The zone in which the driver will be hosted, e.g. de-fra1. Defaults to `nodeHost` zone.")
	flagSet.StringVar(&c.Username, "username", "", "UpCloud username")
	flagSet.StringVar(&c.Password, "password", "", "UpCloud password")
	flagSet.StringVar(&c.DriverName, "driver-name", DefaultDriverName, "Name for the driver")
	flagSet.StringVar(&c.HealtServerAddress, "address", DefaultHealtServerAddress, "Address to serve on")
	flagSet.BoolVar(&c.PrintVersion, "version", false, "Print the version and exit.")
	flagSet.StringVar(&c.Mode, "mode", DefaultDriverMode, "Driver mode, one of node, controller, or monolith.")
	flagSet.StringVar(&c.LogLevel, "log-level", "info", "Logging level: panic, fatal, error, warn, warning, info, debug or trace")
	flagSet.StringSliceVar(&c.Labels, "label", nil, "Apply default labels to all storage devices created by CSI driver, e.g. --label=color=green --label=size=xl")
	flagSet.StringSliceVar(&c.FilesystemTypes, "fs-types", []string{"ext3", "ext4", "xfs"}, "Filesystem types supported by the system")

	if err := flagSet.Parse(osArgs); err != nil {
		return c, err
	}

	if len(c.Labels) == 0 {
		c.Labels = strings.Split(os.Getenv(envStorageLabels), ",")
	}

	if c.Username == "" {
		c.Username = os.Getenv(envUpcloudUsername)
	}
	if c.Password == "" {
		c.Password = os.Getenv(envUpcloudPassword)
	}

	switch c.Mode {
	case DriverModeController, DriverModeMonolith:
		if err := validateControllerConfig(c); err != nil {
			return c, err
		}
	}
	return c, nil
}

func validateControllerConfig(c Config) error {
	if c.Zone == "" && c.NodeHost == "" {
		return errors.New("controller required that zone or valid node host is set")
	}
	return nil
}
