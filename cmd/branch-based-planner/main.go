package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/fluxcd/pkg/runtime/logger"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/go-logr/logr"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	cgoscheme "k8s.io/client-go/kubernetes/scheme"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(cgoscheme.AddToScheme(scheme))
	utilruntime.Must(sourcev1.AddToScheme(scheme))
	utilruntime.Must(sourcev1b2.AddToScheme(scheme))
	utilruntime.Must(infrav1.AddToScheme(scheme))
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

	pollingCtx, pollingCancel := signal.NotifyContext(context.Background(), os.Interrupt)
	informerCtx, informerCancel := signal.NotifyContext(context.Background(), os.Interrupt)

	dynamicClusterClient, clusterClient, err := getClusterClient()
	if err != nil {
		log.Error(err, "failed get cluster clients")
	}

	go func(log logr.Logger) {
		log.Info("Starting polling server")

		if err := startPollingServer(pollingCtx, log, clusterClient, opts); err != nil {
			log.Error(err, "unable to start polling server")
		}

		// Does not matter if it was an error or not, if this routine is done for
		// unknown reasons, stop the other routine too.
		informerCancel()
	}(log.WithName("polling-server"))

	func(log logr.Logger) {
		log.Info("Starting branch-based planner informer")

		if err := startInformer(informerCtx, log, dynamicClusterClient, clusterClient); err != nil {
			log.Error(err, "unable to start branch-based planner informer")
		}

		// Does not matter if it was an error or not, if this routine is done for
		// unknown reasons, stop the other routine too.
		pollingCancel()
	}(log.WithName("informer"))

	<-pollingCtx.Done()
	<-informerCtx.Done()
}
