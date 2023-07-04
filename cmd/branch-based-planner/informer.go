package main

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/weaveworks/tf-controller/internal/informer/bbp"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func startInformer(ctx context.Context, log logr.Logger, dynamicClient *dynamic.DynamicClient, clusterClient client.Client, opts *applicationOptions) error {
	informer, err := bbp.NewInformer(
		dynamicClient,
		bbp.WithLogger(log),
		bbp.WithClusterClient(clusterClient),
		bbp.WithConfigMapRef(opts.pollingConfigMap),
	)
	if err != nil {
		return fmt.Errorf("failed to create informer: %w", err)
	}

	if err := informer.Start(ctx); err != nil {
		return err
	}

	return nil
}
