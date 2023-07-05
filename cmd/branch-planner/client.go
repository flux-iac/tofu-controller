package main

import (
	"fmt"

	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func getClusterClient() (*dynamic.DynamicClient, client.Client, error) {
	clusterConfig, err := config.GetConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("unable to get cluster config: %w", err)
	}

	dynamicClusterClient, err := dynamic.NewForConfig(clusterConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to get dynamic cluster client: %w", err)
	}

	clusterClient, err := client.New(clusterConfig, client.Options{})
	if err != nil {
		return nil, nil, fmt.Errorf("unable to get cluster client: %w", err)
	}

	return dynamicClusterClient, clusterClient, nil
}
