package main

import (
	"log"
	"os"
	"testing"

	"github.com/UpCloudLtd/upcloud-csi/driver"
	"github.com/spf13/pflag"
)

func TestRun(t *testing.T) {
	t.Skip("WIP")
	t.Parallel()
	flagSet := pflag.NewFlagSet("default", pflag.ContinueOnError)

	version := flagSet.Bool("version", false, "Print the version and exit.")

	if *version {
		log.Printf("%s - %s (%s)\n", driver.GetVersion(), driver.GetCommit(), driver.GetTreeState())
		os.Exit(0)
	}
}
