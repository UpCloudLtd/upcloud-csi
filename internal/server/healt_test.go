package server_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/UpCloudLtd/upcloud-csi/internal/logger"
	"github.com/UpCloudLtd/upcloud-csi/internal/plugin/config"
	"github.com/UpCloudLtd/upcloud-csi/internal/server"
	"github.com/stretchr/testify/require"
)

func TestHealthServer(t *testing.T) {
	t.Parallel()

	l := logger.New("info").WithField("package", "server_test")

	const addr string = config.DefaultHealtServerAddress

	srv, err := server.NewHealthServer(addr, l)
	require.NoError(t, err)
	go func() {
		t.Logf("starting HTTP Health server at %s", addr)
		if err := srv.Run(); err != nil {
			t.Log(err)
		}
	}()
	time.Sleep(2 * time.Second)
	res, err := http.Get(fmt.Sprintf("http%s/health", strings.TrimPrefix(addr, "tcp")))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
	srv.Stop(nil)
}
