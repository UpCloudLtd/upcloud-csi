package sanity_test

import (
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path"
	"testing"
	"time"

	"github.com/UpCloudLtd/upcloud-csi/internal/filesystem/mock"
	"github.com/UpCloudLtd/upcloud-csi/internal/plugin"
	"github.com/UpCloudLtd/upcloud-csi/internal/plugin/config"
	"github.com/kubernetes-csi/csi-test/v5/pkg/sanity"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestDriverSanity(t *testing.T) {
	t.Parallel()

	if os.Getenv("UPCLOUD_TEST_USERNAME") == "" || os.Getenv("UPCLOUD_TEST_PASSWORD") == "" || os.Getenv("UPCLOUD_TEST_HOSTNAME") == "" {
		t.Skip("required environment variables are not set to test CSI sanity")
	}
	socket := path.Join(os.TempDir(), fmt.Sprintf("csi-socket-%d.sock", time.Now().Unix()))
	defer os.Remove(socket)

	endpoint, _ := url.Parse(fmt.Sprintf("unix://%s", socket))

	require.NoError(t, runTestDriver(endpoint))

	cfg, err := newTestConfig(endpoint.String())
	require.NoError(t, err)
	defer func() {
		os.RemoveAll(cfg.StagingPath)
		os.RemoveAll(cfg.TargetPath)
	}()

	// wait server to start
	for i := 0; i < 5; i++ {
		t.Logf("waiting plugin server to create socket %s (%d)", socket, i)
		if _, err := os.Stat(socket); err == nil {
			break
		}
		time.Sleep(time.Second)
	}

	sanity.Test(t, cfg)
}

func runTestDriver(endpoint *url.URL) error {
	var err error

	hostname := os.Getenv("UPCLOUD_TEST_HOSTNAME")
	if hostname == "" {
		hostname, err = os.Hostname()
		if err != nil {
			return err
		}
	}

	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)
	logger.SetOutput(io.Discard)

	go func() {
		if err := plugin.Run(config.Config{
			Username:            os.Getenv("UPCLOUD_TEST_USERNAME"),
			Password:            os.Getenv("UPCLOUD_TEST_PASSWORD"),
			DriverName:          config.DefaultDriverName,
			PluginServerAddress: endpoint.String(),
			HealtServerAddress:  "http://127.0.0.1:8080",
			NodeHost:            hostname,
			Zone:                "",
			Filesystem:          mock.NewFilesystem(logger),
			LogLevel:            logger.Level.String(),
			Mode:                config.DriverModeMonolith,
		}); err != nil {
			log.Fatal(err)
		}
	}()
	return err
}

func newTestConfig(endpoint string) (sanity.TestConfig, error) {
	cfg := sanity.NewTestConfig()
	cfg.Address = endpoint
	return cfg, nil
}
