package main

import (
	"context"
	"os"
	"os/signal"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/runtime/logger"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/go-logr/logr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	cgoscheme "k8s.io/client-go/kubernetes/scheme"
)

func init() {
	utilruntime.Must(cgoscheme.AddToScheme(scheme.Scheme))
	utilruntime.Must(sourcev1.AddToScheme(scheme.Scheme))
	utilruntime.Must(infrav1.AddToScheme(scheme.Scheme))
}

var (
	// BuildSHA is the controller version
	BuildSHA string

	// BuildVersion is the controller build version
	BuildVersion string
)

func main() {
	opts := parseFlags()
	log := logger.
		NewLogger(opts.logOptions).
		WithValues("version", BuildVersion, "sha", BuildSHA)
	logger.SetLogger(log)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)

	dynamicClusterClient, clusterClient, err := getClusterClient()
	if err != nil {
		log.Error(err, "failed get cluster clients")
	}

	go func(log logr.Logger) {
		log.Info("Starting polling server")

		if err := startPollingServer(ctx, log, clusterClient, opts); err != nil {
			log.Error(err, "unable to start polling server")
		}

		// Does not matter if it was an error or not, if this routine is done for
		// unknown reasons, stop the other routine too.
		cancel()
	}(log.WithName("polling-server"))

	informerLog := log.WithName("informer")
	informerLog.Info("Starting branch-based planner informer")
	if err := startInformer(ctx, informerLog, dynamicClusterClient, clusterClient, opts); err != nil {
		informerLog.Error(err, "branch-based planner informer failed")
	}
	// once the informer exits, make sure the goroutine above is also
	// cancelled.
	cancel()
}
