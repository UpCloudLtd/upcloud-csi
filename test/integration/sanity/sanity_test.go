package sanity_test

import (
	"fmt"
	"io"
	"log"
	"os"
	"testing"

	"github.com/UpCloudLtd/upcloud-csi/driver"
	"github.com/UpCloudLtd/upcloud-csi/test/integration/sanity/mock"
	"github.com/kubernetes-csi/csi-test/v5/pkg/sanity"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestDriverSanity(t *testing.T) {
	t.Parallel()

	if os.Getenv("UPCLOUD_TEST_USERNAME") == "" || os.Getenv("UPCLOUD_TEST_PASSWORD") == "" || os.Getenv("UPCLOUD_TEST_HOSTNAME") == "" {
		t.Skip("required environment variables are not set to test CSI sanity")
	}
	socket, err := os.CreateTemp(os.TempDir(), "csi-socket-*.sock")
	require.NoError(t, err)
	defer os.Remove(socket.Name())
	endpoint := fmt.Sprintf("unix:///%s", socket.Name())

	_, err = runTestDriver(endpoint)
	require.NoError(t, err)

	cfg, err := newTestConfig(endpoint)
	require.NoError(t, err)
	defer func() {
		os.RemoveAll(cfg.StagingPath)
		os.RemoveAll(cfg.TargetPath)
	}()

	sanity.Test(t, cfg)
}

func runTestDriver(endpoint string) (*driver.Driver, error) {
	var err error

	hostname := os.Getenv("UPCLOUD_TEST_HOSTNAME")
	if hostname == "" {
		hostname, err = os.Hostname()
		if err != nil {
			return nil, err
		}
	}

	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)
	logger.SetOutput(io.Discard)
	drv, err := driver.NewDriver(
		logger,
		driver.WithDriverName(driver.DefaultDriverName),
		driver.WithEndpoint(endpoint),
		driver.WithUsername(os.Getenv("UPCLOUD_TEST_USERNAME")),
		driver.WithPassword(os.Getenv("UPCLOUD_TEST_PASSWORD")),
		driver.WithNodeHost(hostname),
		driver.WithAddress(driver.DefaultAddress),
		driver.WithControllerOn(true),
	)
	if err != nil {
		return nil, err
	}
	drv, err = driver.UseFilesystem(drv, mock.NewFilesystem(logger))

	go func() {
		if err := drv.Run(); err != nil {
			log.Fatal(err)
		}
	}()
	return drv, err
}

func newTestConfig(endpoint string) (sanity.TestConfig, error) {
	cfg := sanity.NewTestConfig()
	cfg.Address = endpoint
	return cfg, nil
}
