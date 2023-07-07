package config

import (
	"os"

	"github.com/spf13/pflag"
)

type Config struct {
	Version                 bool
	Output                  string
	SecretsManifest         bool
	SetupManifest           bool
	RBACManifest            bool
	CRDManifest             bool
	SnapshotWebhookManifest bool
	DriverVersion           string
	UpCloudUsername         string
	UpCloudPassword         string
	LabelClusterID          string
	Zone                    string
}

func Parse(osArgs []string) (Config, error) {
	c := Config{}

	flagSet := pflag.NewFlagSet("default", pflag.ContinueOnError)

	flagSet.BoolVar(&c.Version, "version", false, "Print the version and exit.")
	flagSet.StringVar(&c.Output, "output", "", "Output to file. Defaults to STDOUT.")
	flagSet.BoolVar(&c.SecretsManifest, "secrets", false, "Include secrets manifest.")
	flagSet.BoolVar(&c.SetupManifest, "setup", true, "Include setup manifest.")
	flagSet.BoolVar(&c.RBACManifest, "rbac", true, "Include RBAC manifest.")
	flagSet.BoolVar(&c.CRDManifest, "crd", false, "Include CRD manifest.")
	flagSet.BoolVar(&c.SnapshotWebhookManifest, "snapshot-webhook", false, "Include snapshot webhook manifest.")
	flagSet.StringVar(&c.DriverVersion, "driver-version", "main", "Use specific driver version to render setup manifest.")
	flagSet.StringVar(&c.UpCloudUsername, "upcloud-username", "", "Use UpCloud username to render secrets manifest. If empty, 'UPCLOUD_USERNAME' environment variable is used.")
	flagSet.StringVar(&c.UpCloudPassword, "upcloud-password", "", "Use UpCloud password to render secrets manifest. If empty, 'UPCLOUD_PASSWORD' environment variable is used. Note that plaintext password can be decoded from manifest so store it with care.")
	flagSet.StringVar(&c.LabelClusterID, "label-cluster-id", "", "Apply cluster ID label to all storages created by the driver")
	flagSet.StringVar(&c.Zone, "zone", "de-fra1", "UpCloud zone")

	if err := flagSet.Parse(osArgs); err != nil {
		return c, err
	}

	if c.UpCloudUsername == "" {
		c.UpCloudUsername = os.Getenv("UPCLOUD_USERNAME")
	}

	if c.UpCloudPassword == "" {
		c.UpCloudPassword = os.Getenv("UPCLOUD_PASSWORD")
	}
	return c, nil
}
