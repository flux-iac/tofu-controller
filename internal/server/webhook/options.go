package webhook

import "github.com/go-logr/logr"

type Option func(s *Server) error

func WithLogger(log logr.Logger) Option {
	return func(s *Server) error {
		s.log = log

		return nil
	}
}

func WithListenAddress(addr string) Option {
	return func(s *Server) error {
		s.listenAddr = addr

		return nil
	}
}
