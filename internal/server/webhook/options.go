package webhook

import (
	"fmt"
	"net"

	"github.com/go-logr/logr"
)

type Option func(s *Server) error

func WithLogger(log logr.Logger) Option {
	return func(s *Server) error {
		s.log = log

		return nil
	}
}

func WithListenAddress(addr string) Option {
	return func(s *Server) error {
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("failed creating listener: %w", err)
		}

		s.listener = listener

		return nil
	}
}

func WithListener(listener net.Listener) Option {
	return func(s *Server) error {
		s.listener = listener

		return nil
	}
}
