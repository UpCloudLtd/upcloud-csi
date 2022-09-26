package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"

	. "github.com/UpCloudLtd/upcloud-csi/deploy/kubernetes"
	"github.com/UpCloudLtd/upcloud-csi/driver/objgen"
	"github.com/spf13/pflag"
)

func main() {

	flagSet := pflag.NewFlagSet("default", pflag.ContinueOnError)

	var (
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

	manifest, err := objgen.GetTemplate(vars, templates...)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(manifest.RawYaml))
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
