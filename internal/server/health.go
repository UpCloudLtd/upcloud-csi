package server

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	// healthTimeout specifies a time limit for health HTTP server in seconds.
	healthTimeout = 15
)

type HealthServer struct {
	srv    *http.Server
	log    *logrus.Entry
	listen *url.URL
}

func NewHealthServer(addr string, l *logrus.Entry) (*HealthServer, error) {
	listen, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandlerFn(l))
	return &HealthServer{
		listen: listen,
		log:    l,
		srv: &http.Server{
			Handler:           mux,
			ReadHeaderTimeout: healthTimeout * time.Second,
		},
	}, nil
}

func (s *HealthServer) Run() error {
	s.log.WithFields(logrus.Fields{
		"listen":     s.listen.String(),
		"health_url": fmt.Sprintf("http://%s/health", s.listen.Host),
	}).Info("starting HTTP server")

	listener, err := net.Listen(s.listen.Scheme, s.listen.Host)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	return s.srv.Serve(listener)
}

func (s *HealthServer) Stop(sig os.Signal) {
	s.log.WithField("signal", sig).Info("stopping HTTP server")
	if s.srv != nil {
		if err := s.srv.Close(); err != nil {
			s.log.Error(err)
		}
	}
}

func healthHandlerFn(l *logrus.Entry) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		l.Infof("%s %s %s [%s]", r.Method, r.Host, r.URL.String(), r.UserAgent())
		w.WriteHeader(http.StatusOK)
	}
}
