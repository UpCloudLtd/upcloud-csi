package main

import (
	"errors"
	"log"
	"os"

	"github.com/UpCloudLtd/upcloud-csi/internal/manifest"
	"github.com/UpCloudLtd/upcloud-csi/internal/manifest/config"
	"github.com/UpCloudLtd/upcloud-csi/internal/plugin"
	"github.com/spf13/pflag"
)

func main() {
	c, err := config.Parse(os.Args[1:])
	if err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			os.Exit(0)
		}
		log.Fatalln(err)
	}

	if c.Version {
		plugin.PrintVersion()
		os.Exit(0)
	}
	if err := manifest.Render(c); err != nil {
		log.Fatal(err)
	}
}
