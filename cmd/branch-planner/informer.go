package main

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	tfv1alpha2 "github.com/weaveworks/tf-controller/api/v1alpha2"
	"github.com/weaveworks/tf-controller/internal/config"
	"github.com/weaveworks/tf-controller/internal/git/provider"
	planner "github.com/weaveworks/tf-controller/internal/informer/branch-planner"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func startInformer(ctx context.Context, log logr.Logger, dynamicClient *dynamic.DynamicClient, clusterClient client.Client, opts *applicationOptions) error {
	gitProvider, err := createProvider(ctx, clusterClient, opts.pollingConfigMap)
	if err != nil {
		return fmt.Errorf("failed to create git provider: %w", err)
	}

	sharedInformer, err := createSharedInformer(ctx, clusterClient, dynamicClient)
	if err != nil {
		return fmt.Errorf("failed to create shared informer: %w", err)
	}

	informer, err := planner.NewInformer(
		planner.WithLogger(log),
		planner.WithClusterClient(clusterClient),
		planner.WithGitProvider(gitProvider),
		planner.WithSharedInformer(sharedInformer),
	)
	if err != nil {
		return fmt.Errorf("failed to create informer: %w", err)
	}

	if err := informer.Start(ctx); err != nil {
		return err
	}

	return nil
}

func createProvider(ctx context.Context, clusterClient client.Client, configMapName string) (provider.Provider, error) {
	cmKey, err := config.ObjectKeyFromName(configMapName)
	if err != nil {
		return nil, fmt.Errorf("failed getting object key from config map name: %w", err)
	}

	config, err := config.ReadConfig(ctx, clusterClient, cmKey)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	bbpProviderSecret := &corev1.Secret{}
	if err := clusterClient.Get(ctx, client.ObjectKey{Name: config.SecretName, Namespace: config.SecretNamespace}, bbpProviderSecret); err != nil {
		return nil, fmt.Errorf("unable to get bbp config secret: %w", err)
	}

	gitProvider, err := provider.New(provider.ProviderGitHub, provider.WithToken("api-token", string(bbpProviderSecret.Data["token"])))
	if err != nil {
		return nil, fmt.Errorf("unable to get provider: %w", err)
	}

	return gitProvider, nil
}

func createSharedInformer(_ context.Context, client client.Client, dynamicClient dynamic.Interface) (cache.SharedIndexInformer, error) {
	restMapper := client.RESTMapper()
	mapping, err := restMapper.RESTMapping(tfv1alpha2.GroupVersion.WithKind(tfv1alpha2.TerraformKind).GroupKind())
	if err != nil {
		return nil, fmt.Errorf("failed to look up mapping for CRD: %w", err)
	}

	tweakListOptionsFunc := func(options *metav1.ListOptions) {
		options.LabelSelector = fmt.Sprintf("%s=%s", config.LabelKey, config.LabelValue)
	}

	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, time.Minute, corev1.NamespaceAll, tweakListOptionsFunc)

	return factory.ForResource(mapping.Resource).Informer(), nil
}
