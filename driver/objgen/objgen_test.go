package objgen

import (
	"testing"

	. "github.com/UpCloudLtd/upcloud-csi/deploy/kubernetes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Instantination(t *testing.T) {

	inputVars := map[string]string{

		"UPCLOUD_CSI_USERNAME_B64": "dXNlcg==",
		"UPCLOUD_CSI_PASSWORD_B64": "cGFzcw==",

		"UPCLOUD_CSI_VERSION": "v0.3.1",
	}

	res, err := Get(inputVars)
	require.Nil(t, err)
	require.Zero(t, len(res.UnstructuredObjects))
}

func TestGetTemplate(t *testing.T) {
	want := `
apiVersion: v1
kind: Secret
metadata:
  name: upcloud
  namespace: kube-system
data:
  username: "test_username"
  password: "test_password"`

	vars := map[string]string{
		"UPCLOUD_CSI_USERNAME_B64": "test_username",
		"UPCLOUD_CSI_PASSWORD_B64": "test_password",
	}

	res, err := GetTemplate(vars, SecretsTemplate)
	require.NoError(t, err)
	assert.YAMLEq(t, want, string(res.RawYaml))

	vars = map[string]string{
		"UPCLOUD_CSI_USERNAME_B64": "test_username",
	}
	// this should return error "can't parse template #0: variable UPCLOUD_CSI_PASSWORD_B64 is not defined in input variables"
	_, err = GetTemplate(vars, SecretsTemplate)
	require.Error(t, err)
}
