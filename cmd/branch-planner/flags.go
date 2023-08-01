package main

import (
	"os"
	"time"

	"github.com/fluxcd/pkg/runtime/acl"
	"github.com/fluxcd/pkg/runtime/logger"
	flag "github.com/spf13/pflag"

	"github.com/weaveworks/tf-controller/internal/server/polling"
)

type applicationOptions struct {
	pollingConfigMap      string
	pollingInterval       time.Duration
	branchPollingInterval time.Duration

	allowedNamespaces []string

	logOptions logger.Options

	runtimeNamespace   string
	watchAllNamespaces bool
	watchNamespace     string

	noCrossNamespaceRefs bool
}

func parseFlags() *applicationOptions {
	opts := &applicationOptions{}

	flag.StringVar(&opts.pollingConfigMap,
		"polling-configmap", polling.DefaultConfigMapName,
		"\"Namespace/Name\" of the ConfigMap for the polling service. If Namespace is omitted, runtime namespace will be used.")

	flag.DurationVar(&opts.pollingInterval,
		"polling-interval", polling.DefaultPollingInterval,
		"Wait between two requests to the same Terraform object.")

	flag.DurationVar(&opts.branchPollingInterval,
		"branch-polling-interval", 0,
		"Interval to use for PR branch sources (default is to use the value of --polling-interval).")

	flag.StringSliceVar(&opts.allowedNamespaces,
		"allowed-namespaces",
		[]string{},
		"Allowed namespaced. If it's empty, all namespaces are allowed for the planner. If it's not empty, only resources in the defined namespaces are allowed.")

	opts.logOptions.BindFlags(flag.CommandLine)

	aclOptions := &acl.Options{}
	aclOptions.BindFlags(flag.CommandLine)
	// this flag exists so that the default is to _disallow_ cross-namespace refs. If supplied, it'll override `--no-cross-namespace-refs`; in other words, you can supply `--allow-cross-namespace-refs` with or without a value, and it will be observed.
	var allowCrossNamespaceRefs bool
	flag.BoolVar(&allowCrossNamespaceRefs, "allow-cross-namespace-refs", false,
		"Enable following cross-namespace references. Overrides --no-cross-namespace-refs")

	flag.Parse()

	if opts.branchPollingInterval == 0 {
		opts.branchPollingInterval = opts.pollingInterval
	}

	opts.runtimeNamespace = os.Getenv("RUNTIME_NAMESPACE")

	if !opts.watchAllNamespaces {
		opts.watchNamespace = opts.runtimeNamespace
	}

	// as in ../manager/main.go, this is the case where --no-cross-namespace-refs can be different and not overridden.
	if !flag.CommandLine.Changed("allow-cross-namespace-refs") && flag.CommandLine.Changed("no-cross-namespace-refs") {
		opts.noCrossNamespaceRefs = aclOptions.NoCrossNamespaceRefs
	} else {
		opts.noCrossNamespaceRefs = !allowCrossNamespaceRefs
	}

	return opts
}
