package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"

	. "github.com/UpCloudLtd/upcloud-csi/deploy/kubernetes"
	"github.com/UpCloudLtd/upcloud-csi/driver"
	"github.com/UpCloudLtd/upcloud-csi/driver/objgen"
	"github.com/spf13/pflag"
)

func main() {

	flagSet := pflag.NewFlagSet("default", pflag.ContinueOnError)

	var (
		version                 = flagSet.Bool("version", false, "Print the version and exit.")
		output                  = flagSet.String("output", "", "Output to file. Defaults to STDOUT.")
		secretsManifest         = flagSet.Bool("secrets", false, "Include secrets manifest.")
		setupManifest           = flagSet.Bool("setup", true, "Include setup manifest.")
		rbacManifest            = flagSet.Bool("rbac", true, "Include RBAC manifest.")
		crdManifest             = flagSet.Bool("crd", true, "Include CRD manifest.")
		snapshotWebhookManifest = flagSet.Bool("snapshot-webhook", false, "Include snapshot webhook manifest.")
		driverVersion           = flagSet.String("driver-version", "main", "Use specific driver version to render setup manifest.")
		upcloudUsername         = flagSet.String("upcloud-username", "", "Use UpCloud username to render secrets manifest. If empty, 'UPCLOUD_USERNAME' environment variable is used.")
		upcloudPassword         = flagSet.String("upcloud-password", "", "Use UpCloud password to render secrets manifest. If empty, 'UPCLOUD_PASSWORD' environment variable is used. Note that plaintext password can be decoded from manifest so store it with care.")
	)

	err := flagSet.Parse(os.Args[1:])
	if err != nil {
		if err == pflag.ErrHelp {
			os.Exit(0)
		}
		log.Fatalln(err)
	}

	if *version {
		fmt.Printf("%s - %s (%s)\n", driver.GetVersion(), driver.GetCommit(), driver.GetTreeState())
		os.Exit(0)
	}

	vars := map[string]string{}
	templates := make([]string, 0)
	if *secretsManifest {
		templates = append(templates, SecretsTemplate)
		vars["UPCLOUD_CSI_USERNAME_B64"] = base64.StdEncoding.EncodeToString([]byte(secretsUsername(upcloudUsername)))
		vars["UPCLOUD_CSI_PASSWORD_B64"] = base64.StdEncoding.EncodeToString([]byte(secretsPassword(upcloudPassword)))
	}
	if *crdManifest {
		templates = append(templates, CRDTemplate)
	}
	if *rbacManifest {
		templates = append(templates, RbacTemplate)
	}
	if *setupManifest {
		templates = append(templates, CSITemplate)
		vars["UPCLOUD_CSI_VERSION"] = *driverVersion
	}
	if *snapshotWebhookManifest {
		templates = append(templates, SnapshotTemplate)
	}
	if len(templates) == 0 {
		log.Fatal("select atleast one manifest")
	}

	manifest, err := objgen.Get(vars, templates...)

	if err != nil {
		log.Fatal(err)
	}
	data, err := manifest.MarshalYAML()
	if err != nil {
		log.Fatal(err)
	}
	if err := writeOutput(*output, data); err != nil {
		log.Fatal(err)
	}
}

func writeOutput(file string, data []byte) error {
	if file != "" {
		return os.WriteFile(file, data, 0640)
	}
	_, err := fmt.Println(string(data))
	return err
}

func secretsUsername(username *string) string {
	if username != nil && *username != "" {
		return *username
	}
	return os.Getenv("UPCLOUD_USERNAME")
}

func secretsPassword(password *string) string {
	if password != nil && *password != "" {
		return *password
	}
	return os.Getenv("UPCLOUD_PASSWORD")
}
