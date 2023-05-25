package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/fluxcd/pkg/runtime/logger"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/go-logr/logr"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	"github.com/weaveworks/tf-controller/internal/informer/bbp"
	"github.com/weaveworks/tf-controller/internal/server/webhook"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	cgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
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

	webhookCtx, webhookCancel := signal.NotifyContext(context.Background(), os.Interrupt)
	informerCtx, informerCancel := signal.NotifyContext(context.Background(), os.Interrupt)

	clusterConfig, err := config.GetConfig()
	if err != nil {
		log.Error(err, "unable to get cluster config")
		return
	}

	dynamicClusterClient, err := dynamic.NewForConfig(clusterConfig)
	if err != nil {
		log.Error(err, "unable to get cluster config")
		return
	}

	clusterClient, err := client.New(clusterConfig, client.Options{})
	if err != nil {
		log.Error(err, "unable to get cluster config")
		return
	}

	go func(log logr.Logger) {
		log.Info("Starting webhook server")

		if err := startWebhookServer(webhookCtx, log, clusterClient); err != nil {
			log.Error(err, "unable to start webhook server")
		}

		// Does not matter if it was an error or not, if this routine is done for
		// unknown reasons, stop the other routine too.
		informerCancel()
	}(log.WithName("webhook-server"))

	func(log logr.Logger) {
		log.Info("Starting branch-based planner informer")

		if err := startInformer(informerCtx, log, dynamicClusterClient, clusterClient); err != nil {
			log.Error(err, "unable to start branch-based planner informer")
		}

		// Does not matter if it was an error or not, if this routine is done for
		// unknown reasons, stop the other routine too.
		webhookCancel()
	}(log.WithName("informer"))

	<-webhookCtx.Done()
	<-informerCtx.Done()
}

func startWebhookServer(ctx context.Context, log logr.Logger, clusterClient client.Client) error {
	server, err := webhook.New(webhook.WithLogger(log), webhook.WithClusterClient(clusterClient))
	if err != nil {
		return fmt.Errorf("problem configuring the webhook receiver server: %w", err)
	}

	if err := server.Start(ctx); err != nil {
		return fmt.Errorf("problem running the webhook receiver server: %w", err)
	}

	return nil
}

func startInformer(ctx context.Context, log logr.Logger, dynamicClient *dynamic.DynamicClient, clusterClient client.Client) error {
	informer, err := bbp.NewInformer(log, dynamicClient, clusterClient)
	if err != nil {
		return fmt.Errorf("failed to create informer: %w", err)
	}

	if err := informer.Start(ctx); err != nil {
		return err
	}

	return nil
}
