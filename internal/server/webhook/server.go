package webhook

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
)

const (
	DefaultListenAddress = ":8080"
)

type Server struct {
	log        logr.Logger
	listenAddr string
}

func New(options ...Option) *Server {
	server := &Server{
		log:        logr.Discard(),
		listenAddr: DefaultListenAddress,
	}

	for _, opt := range options {
		opt(server)
	}

	return server
}

func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	mux.Handle("/callback", newCallbackHandler(s.log))
	mux.Handle("/healthz", healthz.CheckHandler{Checker: healthz.Ping})

	listener, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("failed creating listener: %w", err)
	}

	srv := http.Server{
		Addr:    listener.Addr().String(),
		Handler: mux,
	}

	go func() {
		log := s.log.WithValues("kind", "webhook", "addr", listener.Addr())
		log.Info("Starting server")
		if err := srv.Serve(listener); err != nil {
			if errors.Is(err, http.ErrServerClosed) {
				return
			}
			log.Error(err, "failed serving")
		}
	}()

	<-ctx.Done()

	return srv.Shutdown(ctx)
}
