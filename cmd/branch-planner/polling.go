package main

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/weaveworks/tf-controller/internal/server/polling"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func startPollingServer(ctx context.Context, log logr.Logger, clusterClient client.Client, opts *applicationOptions) error {
	server, err := polling.New(
		polling.WithLogger(log),
		polling.WithClusterClient(clusterClient),
		polling.WithConfigMap(opts.pollingConfigMap),
		polling.WithPollingInterval(opts.pollingInterval),
		polling.WithBranchPollingInterval(opts.branchPollingInterval),
	)
	if err != nil {
		return fmt.Errorf("problem configuring the polling server: %w", err)
	}

	if err := server.Start(ctx); err != nil {
		return fmt.Errorf("problem running the polling server: %w", err)
	}

	return nil
}
