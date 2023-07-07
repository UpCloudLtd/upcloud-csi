package manifest_test

import (
	"os"
	"testing"

	"github.com/UpCloudLtd/upcloud-csi/internal/manifest"
	"github.com/UpCloudLtd/upcloud-csi/internal/manifest/config"
	"github.com/stretchr/testify/require"
)

func TestRender(t *testing.T) {
	t.Parallel()

	output, err := os.CreateTemp(os.TempDir(), "upcloud-csi-test-manifest")
	require.NoError(t, err)
	defer os.Remove(output.Name())

	t.Logf("rendering test manifest to temporary file %s", output.Name())
	want := `
apiVersion: v1
kind: Secret
metadata:
  name: upcloud
  namespace: kube-system
  creationTimestamp:
data:
  username: dGVzdA==
  password: cGFzcw==`
	err = manifest.Render(config.Config{
		UpCloudUsername: "test",
		UpCloudPassword: "pass",
		SecretsManifest: true,
		Output:          output.Name(),
	})
	require.NoError(t, err)

	got, err := os.ReadFile(output.Name())
	require.NoError(t, err)
	require.YAMLEq(t, want, string(got))
}
