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
	log      logr.Logger
	listener net.Listener
}

func New(options ...Option) *Server {
	server := &Server{log: logr.Discard()}

	for _, opt := range options {
		opt(server)
	}

	return server
}

func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	mux.Handle("/callback", NewCallbackHandler(s.log))
	mux.Handle("/healthz", healthz.CheckHandler{Checker: healthz.Ping})
	mux.HandleFunc("/", func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusNotFound)
	})

	if s.listener == nil {
		var err error
		s.listener, err = net.Listen("tcp", DefaultListenAddress)
		if err != nil {
			return fmt.Errorf("failed creating listener: %w", err)
		}
	}

	srv := http.Server{
		Addr:    s.listener.Addr().String(),
		Handler: mux,
	}

	go func() {
		log := s.log.WithValues("kind", "webhook", "addr", s.listener.Addr())
		log.Info("Starting server")
		if err := srv.Serve(s.listener); err != nil {
			if errors.Is(err, http.ErrServerClosed) {
				return
			}
			log.Error(err, "failed serving")
		}
	}()

	<-ctx.Done()

	return srv.Shutdown(ctx)
}
