package main_test

import (
	"os"
	"testing"

	log "github.com/sirupsen/logrus"

	"github.com/UpCloudLtd/upcloud-csi/driver"
	"github.com/spf13/pflag"
)

func TestRun(t *testing.T) {
	t.Parallel()
	flagSet := pflag.NewFlagSet("default", pflag.ContinueOnError)

	version := flagSet.Bool("version", false, "Print the version and exit.")

	if *version {
		log.Printf("%s - %s (%s)\n", driver.GetVersion(), driver.GetCommit(), driver.GetTreeState())
		os.Exit(0)
	}

	// Disabled for now as it seems to try to start the proper server (and hangs there as its listening for connections)
	/*
		d := driver.NewMockDriver()


		if err := d.Run(); err != nil {
			log.Fatalln(err)
		}*/
}
