package main

import (
	"os"

	"github.com/fluxcd/pkg/runtime/logger"
	flag "github.com/spf13/pflag"
	"github.com/weaveworks/tf-controller/internal/server/webhook"
)

type applicationOptions struct {
	serverAddr string

	logOptions logger.Options

	runtimeNamespace   string
	watchAllNamespaces bool
	watchNamespace     string
}

func parseFlags() *applicationOptions {
	opts := &applicationOptions{}

	flag.StringVar(&opts.serverAddr,
		"bind-address", webhook.DefaultListenAddress,
		"The address the webhook server endpoint binds to.")

	opts.logOptions.BindFlags(flag.CommandLine)

	flag.Parse()

	opts.runtimeNamespace = os.Getenv("RUNTIME_NAMESPACE")

	if !opts.watchAllNamespaces {
		opts.watchNamespace = opts.runtimeNamespace
	}

	return opts
}
