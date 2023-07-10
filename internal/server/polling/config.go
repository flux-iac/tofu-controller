package polling

import (
	"context"

	"github.com/weaveworks/tf-controller/internal/config"
)

const DefaultConfigMapName = "default/branch-planner"

func (s *Server) readConfig(ctx context.Context) (*config.Config, error) {
	configMap, err := config.ReadConfig(ctx, s.clusterClient, s.configMapRef)

	return &configMap, err
}
