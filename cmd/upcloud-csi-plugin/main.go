package main

import (
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/UpCloudLtd/upcloud-csi/internal/logger"
	"github.com/UpCloudLtd/upcloud-csi/internal/plugin"
	"github.com/UpCloudLtd/upcloud-csi/internal/plugin/config"
	"github.com/spf13/pflag"
)

func main() {
	config, err := config.Parse(os.Args[1:])
	if err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			os.Exit(0)
		}
		log.Fatal(err)
	}
	if config.PrintVersion {
		plugin.PrintVersion()
		os.Exit(0)
	}
	if err := plugin.Run(config); err != nil && !errors.Is(err, http.ErrServerClosed) {
		l := logger.New(config.LogLevel).WithField(logger.ZoneKey, config.Zone)
		l.Error(err)
	}
}
