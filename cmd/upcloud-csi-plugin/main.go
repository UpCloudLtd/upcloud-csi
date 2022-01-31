package main

import (
	"fmt"
	"github.com/UpCloudLtd/upcloud-csi/driver"
	"github.com/spf13/pflag"
	"log"
	"os"
)

func main() {
	flagSet := pflag.NewFlagSet("default", pflag.ContinueOnError)

	var (
		endpoint     = flagSet.String("endpoint", "unix:///var/lib/kubelet/plugins/"+driver.DefaultDriverName+"/csi.sock", "CSI endpoint")
		nodeHost     = flagSet.String("nodehost", "", "Node hostname to determine node's zone and UUID")
		username     = flagSet.String("username", "", "Upcloud username")
		password     = flagSet.String("password", "", "Upcloud password")
		driverName   = flagSet.String("driver-name", driver.DefaultDriverName, "Name for the driver")
		address      = flagSet.String("address", driver.DefaultAddress, "Address to serve on")
		volumeName   = flagSet.String("volume_name", "", "Name for the volume being provisioned by driver")
		version      = flagSet.Bool("version", false, "Print the version and exit.")
		isController = flagSet.Bool("is_controller", true, "Run driver with controller included")
	)

	flagSet.Parse(os.Args[1:])

	if *version {
		fmt.Printf("%s - %s (%s)\n", driver.GetVersion(), driver.GetCommit(), driver.GetTreeState())
		os.Exit(0)
	}

	if *nodeHost == "" {
		log.Fatalln("nodehost missing")
	}

	drv, err := driver.NewDriver(
		driver.WithDriverName(*driverName),
		driver.WithVolumeName(*volumeName),
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
