package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/UpCloudLtd/upcloud-csi/driver"
	"github.com/UpCloudLtd/upcloud-go-api/v6/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v6/upcloud/client"
	"github.com/UpCloudLtd/upcloud-go-api/v6/upcloud/service"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
)

const (
	// clientTimeout helps to tune for timeout on requests to UpCloud API. Measurement: seconds.
	clientTimeout time.Duration = 600 * time.Second

	envUpcloudUsername string = "UPCLOUD_USERNAME"
	envUpcloudPassword string = "UPCLOUD_PASSWORD"
	envStorageLabels   string = "STORAGE_LABELS"
)

func main() {
	flagSet := pflag.NewFlagSet("default", pflag.ContinueOnError)
	var (
		endpoint     = flagSet.String("endpoint", driver.DefaultEndpoint, "CSI endpoint")
		nodeHost     = flagSet.String("nodehost", "", "Node's hostname. This should match server's `hostname` in the hub.upcloud.com.")
		zone         = flagSet.String("zone", "", "The zone in which the driver will be hosted, e.g. de-fra1. Defaults to `nodeHost` zone.")
		username     = flagSet.String("username", "", "UpCloud username")
		password     = flagSet.String("password", "", "UpCloud password")
		driverName   = flagSet.String("driver-name", driver.DefaultDriverName, "Name for the driver")
		address      = flagSet.String("address", driver.DefaultAddress, "Address to serve on")
		version      = flagSet.Bool("version", false, "Print the version and exit.")
		isController = flagSet.Bool("is_controller", true, "Run driver with controller included")
		logLevel     = flagSet.String("log-level", "info", "Logging level: panic, fatal, error, warn, warning, info, debug or trace")
		labels       = flagSet.StringSlice("label", nil, "Apply default labels to all storage devices created by CSI driver, e.g. --label=color=green --label=size=xl")
	)
	if err := flagSet.Parse(os.Args[1:]); err != nil {
		log.Fatal(err)
	}

	if *version {
		printVersionAndExit()
	}

	if *nodeHost == "" {
		log.Fatalf("nodehost missing")
	}
	svc := service.New(newUpcloudClient(*username, *password))
	logger := newLogger(*logLevel)
	options := driver.Options{
		DriverName:    *driverName,
		Endpoint:      newEndpoint(*endpoint),
		Address:       newAddress(*address),
		NodeHost:      *nodeHost,
		Zone:          *zone,
		IsController:  *isController,
		StorageLabels: newLabels(*labels),
	}
	drv, err := driver.NewDriver(svc, options, logger, nil)
	if err != nil {
		logger.Fatal(err)
	}

	if err := drv.Run(driver.NewUpcloudHealthChecker(svc)); err != nil {
		log.Fatalln(err)
	}
}

func printVersionAndExit() {
	fmt.Printf("%s - %s (%s)\n", driver.GetVersion(), driver.GetCommit(), driver.GetTreeState()) //nolint: forbidigo // allow printing to console
	os.Exit(0)
}

func newUpcloudClient(username, password string) *client.Client {
	if username == "" {
		username = os.Getenv(envUpcloudUsername)
	}
	if password == "" {
		password = os.Getenv(envUpcloudPassword)
	}
	return client.New(username, password, client.WithTimeout(clientTimeout))
}

func newAddress(addr string) *url.URL {
	addressURL, err := url.Parse(addr)
	if err != nil {
		log.Fatalf("unable to parse address: %v", err)
	}
	return addressURL
}

func newEndpoint(endpoint string) *url.URL {
	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		log.Fatalf("unable to parse endpoint: %v", err)
	}
	// CSI plugins talk only over UNIX sockets currently
	if endpointURL.Scheme != "unix" {
		log.Fatalf("currently only unix domain sockets are supported, have: %s", endpointURL.Scheme)
	}
	return endpointURL
}

func newLogger(logLevel string) *logrus.Logger {
	lv, err := logrus.ParseLevel(logLevel)
	if err != nil {
		log.Fatal(err)
	}
	logger := logrus.New()
	logger.SetLevel(lv)
	if logger.GetLevel() > logrus.InfoLevel {
		logger.WithField("level", logger.GetLevel().String()).Warn("using log level higher than INFO is not recommended in production")
	}
	return logger
}

func newLabels(labels []string) []upcloud.Label {
	if len(labels) == 0 {
		labels = strings.Split(os.Getenv(envStorageLabels), ",")
	}
	r := make([]upcloud.Label, 0)
	for _, l := range labels {
		if l == "" {
			continue
		}
		c := strings.SplitN(l, "=", 2)
		if len(c) == 2 {
			r = append(r, upcloud.Label{Key: c[0], Value: c[1]})
		}
	}
	return r
}
