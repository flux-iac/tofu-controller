package main

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	planner "github.com/weaveworks/tf-controller/internal/informer/branch-planner"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func startInformer(ctx context.Context, log logr.Logger, dynamicClient *dynamic.DynamicClient, clusterClient client.Client) error {
	informer, err := planner.NewInformer(log, dynamicClient, clusterClient)
	if err != nil {
		return fmt.Errorf("failed to create informer: %w", err)
	}

	if err := informer.Start(ctx); err != nil {
		return err
	}

	return nil
}
