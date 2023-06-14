package polling

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const DefaultConfigMapName = "default/branch-based-planner"

// Example ConfigMap
//
// The secret is a reference to a secret with a 'token' key.
//
// ---
// kind: ConfigMap
// metadata:
//   name: branch-based-planner
// data:
//   # Secret to use to use GitHub API
//   secretNamespace: flux-system
//   secretName: bbp-token
//   # List of Terraform resources
//   resources: |-
//     - namespace: default
//       name: tf1
//     - namespace: default
//       name: tf2
//     - namespace: infra
//       name: tfcore
//     - namespace: team-a
//       name: helloworld-tf

type Config struct {
	Resources       []client.ObjectKey
	SecretNamespace string
	SecretName      string
}

func (s *Server) readConfig(ctx context.Context) (*Config, error) {
	configMap := &corev1.ConfigMap{}
	err := s.clusterClient.Get(ctx, s.configMapRef, configMap)
	if err != nil {
		return nil, fmt.Errorf("unable to get ConfigMap: %w", err)
	}

	config := &Config{}
	config.SecretNamespace = configMap.Data["secretNamespace"]
	config.SecretName = configMap.Data["secretName"]
	resourceData := configMap.Data["resources"]

	if config.SecretNamespace == "" {
		config.SecretNamespace = "default"
	}

	err = yaml.Unmarshal([]byte(resourceData), &config.Resources)
	if err != nil {
		return nil, fmt.Errorf("failed to parse resource list from ConfigMap: %w", err)
	}

	return config, nil
}
