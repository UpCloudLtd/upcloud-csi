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
		endpoint   = flagSet.String("endpoint", "unix:///var/lib/kubelet/plugins/"+driver.DefaultDriverName+"/csi.sock", "CSI endpoint")
		nodeid     = flagSet.String("nodeid", "", "node id")
		username   = flagSet.String("username", "", "Upcloud username")
		password   = flagSet.String("password", "", "Upcloud password")
		url        = flagSet.String("url", "https://api.upcloud.com/", "Upcloud API URL")
		driverName = flagSet.String("driver-name", driver.DefaultDriverName, "Name for the driver")
		address    = flagSet.String("address", driver.DefaultAddress, "Address to serve on")
		version    = flagSet.Bool("version", false, "Print the version and exit.")
	)

	if *version {
		fmt.Printf("%s - %s (%s)\n", driver.GetVersion(), driver.GetCommit(), driver.GetTreeState())
		os.Exit(0)
	}

	if *nodeid == "" {
		log.Fatalln("nodeid missing")
	}

	drv, err := driver.NewDriver(*endpoint, *username, *password, *url, *nodeid, *driverName, *address)
	if err != nil {
		log.Fatalln(err)
	}

	if err := drv.Run(); err != nil {
		log.Fatalln(err)
	}
}

