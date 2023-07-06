package manifest

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"

	"github.com/UpCloudLtd/upcloud-csi/deploy/kubernetes"
	"github.com/UpCloudLtd/upcloud-csi/internal/manifest/config"
	"github.com/UpCloudLtd/upcloud-csi/pkg/objgen"
)

func Render(c config.Config) error {
	vars := map[string]string{}
	templates := make([]string, 0)

	if c.SecretsManifest {
		templates = append(templates, kubernetes.SecretsTemplate)
		vars["UPCLOUD_CSI_USERNAME_B64"] = base64.StdEncoding.EncodeToString([]byte(c.UpcloudUsername))
		vars["UPCLOUD_CSI_PASSWORD_B64"] = base64.StdEncoding.EncodeToString([]byte(c.UpcloudUsername))
	}
	if c.CRDManifest {
		templates = append(templates, kubernetes.CRDTemplate)
	}
	if c.RBACManifest {
		templates = append(templates, kubernetes.RbacTemplate)
	}
	if c.SetupManifest {
		templates = append(templates, kubernetes.CSITemplate)
		vars["UPCLOUD_CSI_VERSION"] = c.DriverVersion
		vars["CLUSTER_ID"] = c.LabelClusterID
		vars["UPCLOUD_ZONE"] = c.Zone
	}
	if c.SnapshotWebhookManifest {
		templates = append(templates, kubernetes.SnapshotTemplate)
	}

	if len(templates) == 0 {
		return errors.New("select atleast one manifest")
	}

	manifest, err := objgen.Get(vars, templates...)
	if err != nil {
		return err
	}
	data, err := manifest.MarshalYAML()
	if err != nil {
		return err
	}
	if err := writeOutput(c.Output, data); err != nil {
		return err
	}
	return nil
}

func writeOutput(file string, data []byte) error {
	if file != "" {
		return os.WriteFile(file, data, 0o600)
	}
	_, err := fmt.Fprintln(os.Stdout, string(data))
	return err
}
