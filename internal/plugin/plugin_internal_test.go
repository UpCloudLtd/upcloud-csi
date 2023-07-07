package plugin

import (
	"testing"

	"github.com/UpCloudLtd/upcloud-csi/internal/logger"
	"github.com/UpCloudLtd/upcloud-csi/internal/plugin/config"
	"github.com/stretchr/testify/require"
)

func TestNewPluginServer(t *testing.T) {
	t.Parallel()

	l := logger.New("error")
	cfg := config.Config{
		Username:            "test-user",
		Password:            "test-password",
		LogLevel:            "info",
		Mode:                config.DriverModeController,
		Zone:                "fi-hel2",
		PluginServerAddress: config.DefaultPluginServerAddress,
	}
	srv, err := newPluginServer(cfg, l.WithField("package", "plugin"))
	require.NoError(t, err)
	require.Contains(t, srv.GetServiceInfo(), "csi.v1.Controller")
	require.Contains(t, srv.GetServiceInfo(), "csi.v1.Identity")

	cfg = config.Config{
		LogLevel:            "info",
		Mode:                config.DriverModeNode,
		NodeHost:            hostname(),
		PluginServerAddress: config.DefaultPluginServerAddress,
		Zone:                "fi-hel2",
	}
	srv, err = newPluginServer(cfg, l.WithField("package", "plugin"))
	require.NoError(t, err)
	require.Contains(t, srv.GetServiceInfo(), "csi.v1.Node")
	require.Contains(t, srv.GetServiceInfo(), "csi.v1.Identity")

	cfg = config.Config{
		Username:            "test-user",
		Password:            "test-password",
		LogLevel:            "info",
		Mode:                config.DriverModeMonolith,
		NodeHost:            hostname(),
		PluginServerAddress: config.DefaultPluginServerAddress,
		Zone:                "fi-hel2",
	}
	srv, err = newPluginServer(cfg, l.WithField("package", "plugin"))
	require.NoError(t, err)
	require.Contains(t, srv.GetServiceInfo(), "csi.v1.Node")
	require.Contains(t, srv.GetServiceInfo(), "csi.v1.Identity")
	require.Contains(t, srv.GetServiceInfo(), "csi.v1.Controller")
}
