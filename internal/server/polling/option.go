package polling

import (
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/weaveworks/tf-controller/internal/config"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Option func(s *Server) error

func WithLogger(log logr.Logger) Option {
	return func(s *Server) error {
		s.log = log

		return nil
	}
}

func WithClusterClient(clusterClient client.Client) Option {
	return func(s *Server) error {
		s.clusterClient = clusterClient

		return nil
	}
}

func WithConfigMap(configMapName string) Option {
	return func(s *Server) error {
		key, err := config.ObjectKeyFromName(configMapName)
		if err != nil {
			return fmt.Errorf("failed getting object key from config map name: %w", err)
		}

		s.configMapRef = client.ObjectKey{
			Namespace: key.Namespace,
			Name:      key.Name,
		}

		return nil
	}
}

func WithPollingInterval(interval time.Duration) Option {
	return func(s *Server) error {
		s.pollingInterval = interval

		return nil
	}
}

func WithBranchPollingInterval(interval time.Duration) Option {
	return func(s *Server) error {
		s.branchPollingInterval = interval

		return nil
	}
}

func WithNoCrossNamespaceRefs(deny bool) Option {
	return func(s *Server) error {
		s.noCrossNamespaceRefs = deny
		return nil
	}
}
