package main

import (
	"context"
	"fmt"
	"time"

	tfv1alpha2 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/internal/config"
	"github.com/flux-iac/tofu-controller/internal/git/provider"
	planner "github.com/flux-iac/tofu-controller/internal/informer/branch-planner"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func startInformer(ctx context.Context, log logr.Logger, dynamicClient *dynamic.DynamicClient, clusterClient client.Client, opts *applicationOptions) error {
	providerOpts, err := getProviderOpts(ctx, clusterClient, opts.pollingConfigMap)
	if err != nil {
		return fmt.Errorf("failed to get provider options: %w", err)
	}

	sharedInformer, err := createSharedInformer(ctx, clusterClient, dynamicClient)
	if err != nil {
		return fmt.Errorf("failed to create shared informer: %w", err)
	}

	informer, err := planner.NewInformer(
		planner.WithLogger(log),
		planner.WithClusterClient(clusterClient),
		planner.WithProviderOpts(providerOpts...),
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

func getProviderOpts(ctx context.Context, clusterClient client.Client, configMapName string) ([]provider.ProviderOption, error) {
	cmKey, err := config.ObjectKeyFromName(configMapName)
	if err != nil {
		return nil, fmt.Errorf("failed getting object key from config map name: %w", err)
	}

	cfg, err := config.ReadConfig(ctx, clusterClient, cmKey)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	bbpProviderSecret := &corev1.Secret{}
	if err := clusterClient.Get(ctx, client.ObjectKey{Name: cfg.SecretName, Namespace: cfg.SecretNamespace}, bbpProviderSecret); err != nil {
		return nil, fmt.Errorf("unable to get bbp config secret: %w", err)
	}

	if bbpProviderSecret.Data == nil {
		return nil, fmt.Errorf("provider secret has no data")
	}

	return provider.OptsFromSecret(bbpProviderSecret.Data)
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
