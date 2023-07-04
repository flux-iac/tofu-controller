package polling

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
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
		namespace := "default"
		name := ""
		parts := strings.SplitN(configMapName, "/", 2)

		if len(parts) < 1 {
			return fmt.Errorf("invalid ConfigMap reference: %q", configMapName)
		}

		if len(parts) < 2 {
			name = parts[0]
		} else {
			namespace = parts[0]
			name = parts[1]
		}

		if name == "" || namespace == "" {
			return fmt.Errorf("invalid ConfigMap reference: %q", configMapName)
		}

		s.configMapRef = client.ObjectKey{
			Namespace: namespace,
			Name:      name,
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
