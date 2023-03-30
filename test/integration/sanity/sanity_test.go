package sanity_test

import (
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/UpCloudLtd/upcloud-csi/driver"
	"github.com/UpCloudLtd/upcloud-csi/test/integration/sanity/mock"
	"github.com/UpCloudLtd/upcloud-go-api/v6/upcloud/client"
	"github.com/UpCloudLtd/upcloud-go-api/v6/upcloud/service"
	"github.com/kubernetes-csi/csi-test/v5/pkg/sanity"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

const (
	clientTimeout = 120
)

func TestDriverSanity(t *testing.T) {
	t.Parallel()

	if os.Getenv("UPCLOUD_TEST_USERNAME") == "" || os.Getenv("UPCLOUD_TEST_PASSWORD") == "" || os.Getenv("UPCLOUD_TEST_HOSTNAME") == "" {
		t.Skip("required environment variables are not set to test CSI sanity")
	}
	socket, err := os.CreateTemp(os.TempDir(), "csi-socket-*.sock")
	require.NoError(t, err)
	defer os.Remove(socket.Name())
	endpoint, _ := url.Parse(fmt.Sprintf("unix:///%s", socket.Name()))

	_, err = runTestDriver(endpoint)
	require.NoError(t, err)

	cfg, err := newTestConfig(endpoint.String())
	require.NoError(t, err)
	defer func() {
		os.RemoveAll(cfg.StagingPath)
		os.RemoveAll(cfg.TargetPath)
	}()

	sanity.Test(t, cfg)
}

func runTestDriver(endpoint *url.URL) (*driver.Driver, error) {
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
	c := client.New(
		os.Getenv("UPCLOUD_TEST_USERNAME"),
		os.Getenv("UPCLOUD_TEST_PASSWORD"),
		client.WithTimeout((time.Second * clientTimeout)))

	drv, err := driver.NewDriver(
		service.New(c),
		driver.Options{
			DriverName:   driver.DefaultDriverName,
			Endpoint:     endpoint,
			Address:      nil,
			NodeHost:     hostname,
			Zone:         "",
			IsController: true,
		},
		logger,
		mock.NewFilesystem(logger),
	)
	if err != nil {
		return nil, err
	}
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
