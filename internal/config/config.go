package config

import (
	"context"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	LabelKey                = "infra.weave.works/branch-planner"
	LabelValue              = "true"
	LabelPRIDKey            = "infra.weave.works/pr-id"
	LabelPrimaryResourceKey = "infra.weave.works/primary-resource"
	AnnotationCommentIDKey  = "infra.weave.works/comment-id"
	AnnotationErrorRevision = "infra.weave.works/error-revision"

	// DefaultNamespace will be used if RUNTIME_NAMESPACE is not defined.
	DefaultNamespace = "flux-system"
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
//     - namespace: flux-system
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

	if ref.Namespace == "" {
		ref.Namespace = RuntimeNamespace()
	}

	configMap := &corev1.ConfigMap{}
	err := clusterClient.Get(ctx, ref, configMap)
	if err != nil {
		return Config{}, fmt.Errorf("unable to get ConfigMap: %w", err)
	}

	config.SecretNamespace = configMap.Data["secretNamespace"]
	config.SecretName = configMap.Data["secretName"]
	resourceData := configMap.Data["resources"]

	if config.SecretNamespace == "" {
		config.SecretNamespace = RuntimeNamespace()
	}

	err = yaml.Unmarshal([]byte(resourceData), &config.Resources)
	if err != nil {
		return config, fmt.Errorf("failed to parse resource list from ConfigMap: %w", err)
	}

	// Set namespace to default namespace if empty.
	for idx := range config.Resources {
		if config.Resources[idx].Namespace == "" {
			config.Resources[idx].Namespace = RuntimeNamespace()
		}
	}

	return config, nil
}

func ObjectKeyFromName(configMapName string) (client.ObjectKey, error) {
	key := client.ObjectKey{}
	namespace := RuntimeNamespace()
	name := ""
	parts := strings.Split(configMapName, "/")

	switch len(parts) {
	case 1:
		name = parts[0]
	case 2:
		namespace = parts[0]
		name = parts[1]
	default:
		return key, fmt.Errorf("invalid ConfigMap reference: %q", configMapName)
	}

	if name == "" || namespace == "" {
		return key, fmt.Errorf("invalid ConfigMap reference: %q", configMapName)
	}

	key.Namespace = namespace
	key.Name = name

	return key, nil
}

func RuntimeNamespace() string {
	if value := os.Getenv("RUNTIME_NAMESPACE"); value != "" {
		return value
	}

	return DefaultNamespace
}
