package main

import (
	"fmt"
	"log"
	"os"

	"github.com/UpCloudLtd/upcloud-csi/driver"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
)

func main() {
	flagSet := pflag.NewFlagSet("default", pflag.ContinueOnError)

	var (
		endpoint     = flagSet.String("endpoint", "unix:///var/lib/kubelet/plugins/"+driver.DefaultDriverName+"/csi.sock", "CSI endpoint")
		nodeHost     = flagSet.String("nodehost", "", "Node hostname to determine node's zone and UUID")
		username     = flagSet.String("username", "", "UpCloud username")
		password     = flagSet.String("password", "", "UpCloud password")
		driverName   = flagSet.String("driver-name", driver.DefaultDriverName, "Name for the driver")
		address      = flagSet.String("address", driver.DefaultAddress, "Address to serve on")
		version      = flagSet.Bool("version", false, "Print the version and exit.")
		isController = flagSet.Bool("is_controller", true, "Run driver with controller included")
		logLevel     = flagSet.String("log-level", "warning", "Loggin level: panic, fatal, error, warn, warning, info, debug or trace")
	)

	err := flagSet.Parse(os.Args[1:])
	if err != nil {
		log.Fatalln(err)
	}

	if *username == "" {
		*username = os.Getenv("USERNAME")
	}
	if *password == "" {
		*password = os.Getenv("PASSWORD")
	}

	if *version {
		fmt.Printf("%s - %s (%s)\n", driver.GetVersion(), driver.GetCommit(), driver.GetTreeState()) //nolint: forbidigo // allow printing to console
		os.Exit(0)
	}

	if *nodeHost == "" {
		log.Fatalln("nodehost missing")
	}

	logger, err := newLogger(*logLevel)
	if err != nil {
		log.Fatal(err)
	}

	drv, err := driver.NewDriver(
		logger,
		driver.WithDriverName(*driverName),
		driver.WithEndpoint(*endpoint),
		driver.WithUsername(*username),
		driver.WithPassword(*password),
		driver.WithNodeHost(*nodeHost),
		driver.WithAddress(*address),
		driver.WithControllerOn(*isController),
	)
	if err != nil {
		log.Fatalln(err)
	}

	if err := drv.Run(); err != nil {
		log.Fatalln(err)
	}
}

func newLogger(logLevel string) (*logrus.Logger, error) {
	lv, err := logrus.ParseLevel(logLevel)
	if err != nil {
		return nil, err
	}
	logger := logrus.New()
	logger.SetLevel(lv)
	if logger.GetLevel() > logrus.InfoLevel {
		logger.WithField("level", logger.GetLevel().String()).Warn("using log level higher than INFO is not recommended in production")
	}
	return logger, nil
}
