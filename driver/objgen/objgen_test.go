package objgen

import (
	"testing"

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
