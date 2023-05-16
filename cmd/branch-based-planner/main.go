package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

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
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/fluxcd/pkg/runtime/logger"
)

const controllerName = "tf-bbp-controller"

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(cgoscheme.AddToScheme(scheme))
	utilruntime.Must(sourcev1.AddToScheme(scheme))
	utilruntime.Must(sourcev1b2.AddToScheme(scheme))
	utilruntime.Must(infrav1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
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

	clusterConfig, err := getClusterConfig()
	if err != nil {
		log.Error(err, "unable to get cluster config")
		return
	}

	clusterClient, err := dynamic.NewForConfig(clusterConfig)
	if err != nil {
		log.Error(err, "unable to get cluster config")
		return
	}

	go func(log logr.Logger) {
		log.Info("Starting webhook server")

		if err := startWebhookServer(webhookCtx, log); err != nil {
			log.Error(err, "unable to start webhook server")
		}

		// Does not matter if it was an error or not, if this routine is done for
		// unknown reasons, stop the other routine too.
		informerCancel()
	}(log.WithName("webhook-server"))

	func(log logr.Logger) {
		log.Info("Starting branch-based planner informer")

		if err := startInformer(informerCtx, log, clusterClient); err != nil {
			log.Error(err, "unable to start branch-based planner informer")
		}

		// Does not matter if it was an error or not, if this routine is done for
		// unknown reasons, stop the other routine too.
		webhookCancel()
	}(log.WithName("informer"))

	<-webhookCtx.Done()
	<-informerCtx.Done()
}

func startWebhookServer(ctx context.Context, log logr.Logger) error {
	server, err := webhook.New(webhook.WithLogger(log))
	if err != nil {
		return fmt.Errorf("problem configuring the webhook receiver server: %w", err)
	}

	if err := server.Start(ctx); err != nil {
		return fmt.Errorf("problem running the webhook receiver server: %w", err)
	}

	return nil
}

func startInformer(ctx context.Context, log logr.Logger, client *dynamic.DynamicClient) error {
	informer := bbp.NewInformer(client, log)

	if err := informer.Start(ctx); err != nil {
		return err
	}

	return nil
}

// This function is here mostly so I can start and test while we don't have an
// in-cluster Tilefile or something similar.
func getClusterConfig() (*rest.Config, error) {
	kubeConfig := os.Getenv("KUBE_CONFIG")

	if kubeConfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeConfig)
	} else {
		return rest.InClusterConfig()
	}

}
