package objgen

import (
	"testing"

	. "github.com/UpCloudLtd/upcloud-csi/deploy/kubernetes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
)

func TestGet_Instantination(t *testing.T) {
	t.Parallel()
	res, err := Get(map[string]string{
		"UPCLOUD_CSI_USERNAME_B64": "dXNlcg==",
		"UPCLOUD_CSI_PASSWORD_B64": "cGFzcw==",
		"UPCLOUD_CSI_VERSION":      "v0.3.1",
	})
	require.NoError(t, err)
	require.Zero(t, len(res.UnstructuredObjects))
	data, err := res.MarshalYAML()
	require.NoError(t, err)
	require.NotEmpty(t, data)
	kindCounts := map[string]int{
		"ClusterRole":              6,
		"ClusterRoleBinding":       6,
		"CSIDriver":                1,
		"CustomResourceDefinition": 3,
		"DaemonSet":                1,
		"Deployment":               1,
		"Role":                     2,
		"RoleBinding":              2,
		"ServiceAccount":           2,
		"StatefulSet":              1,
		"StorageClass":             2,
		"VolumeSnapshotClass":      1,
		"Secret":                   1,
	}

	for _, o := range res.Objects {
		kind := o.GetObjectKind().GroupVersionKind().Kind
		if _, ok := kindCounts[kind]; ok {
			kindCounts[kind] -= 1
		} else {
			t.Errorf("objects doesn't contain kind '%s'", kind)
		}
	}
	for key, val := range kindCounts {
		if val != 0 {
			t.Errorf("kind '%s' delta mismatch want 0 got %+d", key, val)
		}
	}
}

func TestGet_Template(t *testing.T) {
	t.Parallel()
	want := `
apiVersion: v1
kind: Secret
metadata:
  name: upcloud
  namespace: kube-system
  creationTimestamp: 
data:
  username: "dGVzdF91c2Vy"
  password: "dGVzdF9wYXNzd29yZA=="`

	vars := map[string]string{
		"UPCLOUD_CSI_USERNAME_B64": "dGVzdF91c2Vy",
		"UPCLOUD_CSI_PASSWORD_B64": "dGVzdF9wYXNzd29yZA==",
	}

	res, err := Get(vars, SecretsTemplate)
	require.NoError(t, err)
	data, err := res.MarshalYAML()
	require.NoError(t, err)
	assert.YAMLEq(t, want, string(data))
	assert.Equal(t, "Secret", res.Objects[0].GetObjectKind().GroupVersionKind().Kind)
	secret, ok := res.Objects[0].(*v1.Secret)
	assert.True(t, ok)
	assert.Equal(t, "upcloud", secret.ObjectMeta.Name)

	vars = map[string]string{
		"UPCLOUD_CSI_USERNAME_B64": "test_username",
	}
	// this should return error "can't parse template #0: variable UPCLOUD_CSI_PASSWORD_B64 is not defined in input variables"
	_, err = Get(vars, SecretsTemplate)
	require.Error(t, err)
}
