package server

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sync/errgroup"
)

type Server interface {
	Run() error
	Stop(os.Signal)
}

func Run(servers ...Server) error {
	var eg errgroup.Group
	for i := range servers {
		server := servers[i]
		eg.Go(func() error {
			return server.Run()
		})
	}

	go listenSyscalls(servers...)
	return eg.Wait()
}

func listenSyscalls(servers ...Server) {
	// Listen for syscall signals for process to interrupt/quit
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	sig := <-sigCh
	signal.Stop(sigCh)
	for i := range servers {
		servers[i].Stop(sig)
	}
	close(sigCh)
}
