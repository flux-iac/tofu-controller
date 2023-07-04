package main

import (
	"os"
	"time"

	"github.com/fluxcd/pkg/runtime/logger"
	flag "github.com/spf13/pflag"
	"github.com/weaveworks/tf-controller/internal/server/polling"
)

type applicationOptions struct {
	pollingConfigMap      string
	pollingInterval       time.Duration
	branchPollingInterval time.Duration

	logOptions logger.Options

	runtimeNamespace   string
	watchAllNamespaces bool
	watchNamespace     string
}

func parseFlags() *applicationOptions {
	opts := &applicationOptions{}

	flag.StringVar(&opts.pollingConfigMap,
		"polling-configmap", polling.DefaultConfigMapName,
		"Namespace and name of the ConfigMap for the polling service.")

	flag.DurationVar(&opts.pollingInterval,
		"polling-interval", polling.DefaultPollingInterval,
		"Wait between two requests to the same Terraform object.")

	flag.DurationVar(&opts.branchPollingInterval,
		"branch-polling-interval", 0,
		"Interval to use for PR branch sources (default is to use the value of --polling-interval).")

	opts.logOptions.BindFlags(flag.CommandLine)

	flag.Parse()

	if opts.branchPollingInterval == 0 {
		opts.branchPollingInterval = opts.pollingInterval
	}

	opts.runtimeNamespace = os.Getenv("RUNTIME_NAMESPACE")

	if !opts.watchAllNamespaces {
		opts.watchNamespace = opts.runtimeNamespace
	}

	return opts
}
