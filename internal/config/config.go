package config

import (
	"context"
	"fmt"
	"strings"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Example ConfigMap
//
// The secret is a reference to a secret with a 'token' key.
//
// ---
// apiVersion: v1
// kind: ConfigMap
// metadata:
//   name: branch-based-planner
// data:
//   # Secret to use to use GitHub API.
//   # Key in the secret: token
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
	Labels          map[string]string
}

func ReadConfig(ctx context.Context, clusterClient client.Client, ref types.NamespacedName) (Config, error) {
	config := Config{}

	configMap := &corev1.ConfigMap{}
	err := clusterClient.Get(ctx, ref, configMap)
	if err != nil {
		return Config{}, fmt.Errorf("unable to get ConfigMap: %w", err)
	}

	config.SecretNamespace = configMap.Data["secretNamespace"]
	config.SecretName = configMap.Data["secretName"]
	resourceData := configMap.Data["resources"]

	if config.SecretNamespace == "" {
		config.SecretNamespace = "default"
	}

	err = yaml.Unmarshal([]byte(resourceData), &config.Resources)
	if err != nil {
		return config, fmt.Errorf("failed to parse resource list from ConfigMap: %w", err)
	}

	return config, nil
}

func ObjectKeyFromName(configMapName string) (client.ObjectKey, error) {
	key := client.ObjectKey{}
	namespace := "default"
	name := ""
	parts := strings.SplitN(configMapName, "/", 2)

	if len(parts) < 1 {
		return key, fmt.Errorf("invalid ConfigMap reference: %q", configMapName)
	}

	if len(parts) < 2 {
		name = parts[0]
	} else {
		namespace = parts[0]
		name = parts[1]
	}

	if name == "" || namespace == "" {
		return key, fmt.Errorf("invalid ConfigMap reference: %q", configMapName)
	}

	key.Namespace = namespace
	key.Name = name

	return key, nil
}
