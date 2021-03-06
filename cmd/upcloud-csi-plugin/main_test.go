package main

import (
	"fmt"
	"github.com/UpCloudLtd/upcloud-csi/driver"
	"github.com/spf13/pflag"
	"log"
	"os"
	"testing"
)

func TestRun(t *testing.T) {
	flagSet := pflag.NewFlagSet("default", pflag.ContinueOnError)

	var (
		version = flagSet.Bool("version", false, "Print the version and exit.")
	)

	if *version {
		fmt.Printf("%s - %s (%s)\n", driver.GetVersion(), driver.GetCommit(), driver.GetTreeState())
		os.Exit(0)
	}

	d := driver.NewMockDriver()

	if err := d.Run(); err != nil {
		log.Fatalln(err)
	}

}
