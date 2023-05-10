package main

import (
	"os"
	"time"

	"github.com/fluxcd/pkg/runtime/client"
	"github.com/fluxcd/pkg/runtime/leaderelection"
	"github.com/fluxcd/pkg/runtime/logger"
	flag "github.com/spf13/pflag"
	"github.com/weaveworks/tf-controller/internal/server/webhook"
)

type applicationOptions struct {
	eventsAddr  string
	healthAddr  string
	metricsAddr string
	serverAddr  string

	clientOptions         client.Options
	leaderElectionOptions leaderelection.Options

	httpRetry  int
	concurrent int

	logOptions logger.Options

	runtimeNamespace   string
	watchAllNamespaces bool
	watchNamespace     string

	caValidityDuration     time.Duration
	certValidityDuration   time.Duration
	rotationCheckFrequency time.Duration
}

func parseFlags() *applicationOptions {
	opts := &applicationOptions{}

	flag.StringVar(&opts.metricsAddr,
		"metrics-addr", ":8080",
		"The address the metric endpoint binds to.")
	flag.StringVar(&opts.eventsAddr,
		"events-addr", "",
		"The address of the events receiver.")
	flag.StringVar(&opts.healthAddr,
		"health-addr", ":9440",
		"The address the health endpoint binds to.")
	flag.StringVar(&opts.serverAddr,
		"bind-address", webhook.DefaultListenAddress,
		"The address the webhook server endpoint binds to.")
	flag.DurationVar(&opts.caValidityDuration,
		"ca-cert-validity-duration", 24*7*time.Hour,
		"The duration that the ca certificate certificates should be valid for. Default is 1 week.")
	flag.DurationVar(&opts.certValidityDuration,
		"cert-validity-duration", 6*time.Hour,
		"(Deprecated) The duration that the mTLS certificate that the runner pod should be valid for.")
	flag.DurationVar(&opts.rotationCheckFrequency,
		"cert-rotation-check-frequency", 30*time.Minute,
		"The interval that the mTLS certificate rotator should check the certificate validity.")
	flag.IntVar(&opts.concurrent,
		"concurrent", 4,
		"The number of concurrent terraform reconciles.")
	flag.IntVar(&opts.httpRetry,
		"http-retry", 9,
		"The maximum number of retries when failing to fetch artifacts over HTTP.")

	opts.clientOptions.BindFlags(flag.CommandLine)
	opts.leaderElectionOptions.BindFlags(flag.CommandLine)
	opts.logOptions.BindFlags(flag.CommandLine)

	flag.Parse()

	opts.runtimeNamespace = os.Getenv("RUNTIME_NAMESPACE")

	if !opts.watchAllNamespaces {
		opts.watchNamespace = opts.runtimeNamespace
	}

	return opts
}
