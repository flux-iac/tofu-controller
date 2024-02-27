/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"os"
	"time"

	"github.com/flux-iac/tofu-controller/mtls"
	"github.com/flux-iac/tofu-controller/runner"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/controllers"
	"github.com/fluxcd/pkg/runtime/acl"
	"github.com/fluxcd/pkg/runtime/client"
	runtimeCtrl "github.com/fluxcd/pkg/runtime/controller"
	"github.com/fluxcd/pkg/runtime/events"
	"github.com/fluxcd/pkg/runtime/leaderelection"
	"github.com/fluxcd/pkg/runtime/logger"
	"github.com/fluxcd/pkg/runtime/metrics"
	"github.com/fluxcd/pkg/runtime/pprof"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"
	flag "github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	ctrl "sigs.k8s.io/controller-runtime"
	//+kubebuilder:scaffold:imports
)

const controllerName = "tf-controller"

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

var (
	// BuildSHA is the controller version
	BuildSHA string

	// BuildVersion is the controller build version
	BuildVersion string
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(sourcev1.AddToScheme(scheme))
	utilruntime.Must(sourcev1b2.AddToScheme(scheme))
	utilruntime.Must(infrav1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var (
		metricsAddr               string
		eventsAddr                string
		healthAddr                string
		concurrent                int
		requeueDependency         time.Duration
		clientOptions             client.Options
		logOptions                logger.Options
		leaderElectionOptions     leaderelection.Options
		watchAllNamespaces        bool
		httpRetry                 int
		caValidityDuration        time.Duration
		certValidityDuration      time.Duration
		rotationCheckFrequency    time.Duration
		runnerGRPCPort            int
		runnerCreationTimeout     time.Duration
		runnerGRPCMaxMessageSize  int
		allowBreakTheGlass        bool
		clusterDomain             string
		aclOptions                acl.Options
		allowCrossNamespaceRefs   bool
		usePodSubdomainResolution bool
	)

	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&eventsAddr, "events-addr", "", "The address of the events receiver.")
	flag.StringVar(&healthAddr, "health-addr", ":9440", "The address the health endpoint binds to.")
	flag.IntVar(&concurrent, "concurrent", 4, "The number of concurrent terraform reconciles.")
	flag.DurationVar(&requeueDependency, "requeue-dependency", 30*time.Second, "The interval at which failing dependencies are reevaluated.")
	flag.BoolVar(&watchAllNamespaces, "watch-all-namespaces", true,
		"Watch for custom resources in all namespaces, if set to false it will only watch the runtime namespace.")
	flag.IntVar(&httpRetry, "http-retry", 9, "The maximum number of retries when failing to fetch artifacts over HTTP.")
	flag.DurationVar(&caValidityDuration, "ca-cert-validity-duration", 24*7*time.Hour,
		"The duration that the ca certificate certificates should be valid for. Default is 1 week.")
	flag.DurationVar(&certValidityDuration, "cert-validity-duration", 6*time.Hour,
		"(Deprecated) The duration that the mTLS certificate that the runner pod should be valid for.")
	flag.DurationVar(&rotationCheckFrequency, "cert-rotation-check-frequency", 30*time.Minute,
		"The interval that the mTLS certificate rotator should check the certificate validity.")
	flag.IntVar(&runnerGRPCPort, "runner-grpc-port", 30000, "The port which will be exposed on the runner pod for gRPC connections.")
	flag.DurationVar(&runnerCreationTimeout, "runner-creation-timeout", 120*time.Second, "Timeout for creating a runner pod.")
	flag.IntVar(&runnerGRPCMaxMessageSize, "runner-grpc-max-message-size", 4, "The maximum message size for gRPC connections in MiB.")
	flag.BoolVar(&allowBreakTheGlass, "allow-break-the-glass", false, "Allow break the glass mode.")
	flag.StringVar(&clusterDomain, "cluster-domain", "cluster.local", "The cluster domain used by the cluster.")
	flag.BoolVar(&usePodSubdomainResolution, "use-pod-subdomain-resolution", false, "Allow to use pod hostname/subdomain DNS resolution instead of IP based")

	clientOptions.BindFlags(flag.CommandLine)
	logOptions.BindFlags(flag.CommandLine)
	leaderElectionOptions.BindFlags(flag.CommandLine)
	// this adds the flag `--no-cross-namespace-refs`, for backward-compatibility of deployments that use that Flux-like flag.
	aclOptions.BindFlags(flag.CommandLine)
	// this flag exists so that the default is to _disallow_ cross-namespace refs. If supplied, it'll override `--no-cross-namespace-refs`; in other words, you can supply `--allow-cross-namespace-refs` with or without a value, and it will be observed.
	flag.BoolVar(&allowCrossNamespaceRefs, "allow-cross-namespace-refs", false,
		"Enable following cross-namespace references. Overrides --no-cross-namespace-refs")

	flag.Parse()

	ctrl.SetLogger(logger.NewLogger(logOptions))

	runtimeNamespace := os.Getenv("RUNTIME_NAMESPACE")

	watchNamespace := ""
	if !watchAllNamespaces {
		watchNamespace = runtimeNamespace
	}

	mgrConfig := ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress:   metricsAddr,
			ExtraHandlers: pprof.GetHandlers(),
		},
		HealthProbeBindAddress:        healthAddr,
		LeaderElection:                leaderElectionOptions.Enable,
		LeaderElectionReleaseOnCancel: leaderElectionOptions.ReleaseOnCancel,
		LeaseDuration:                 &leaderElectionOptions.LeaseDuration,
		RenewDeadline:                 &leaderElectionOptions.RenewDeadline,
		RetryPeriod:                   &leaderElectionOptions.RetryPeriod,
		LeaderElectionID:              "1953de50.contrib.fluxcd.io",
		Logger:                        ctrl.Log,
	}

	if watchNamespace != "" {
		mgrConfig.Cache.DefaultNamespaces = map[string]ctrlcache.Config{
			watchNamespace: ctrlcache.Config{},
		}
	}

	restConfig := client.GetConfigOrDie(clientOptions)
	mgr, err := ctrl.NewManager(restConfig, mgrConfig)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	var eventRecorder *events.Recorder
	if eventRecorder, err = events.NewRecorder(mgr, ctrl.Log, eventsAddr, controllerName); err != nil {
		setupLog.Error(err, "unable to create event recorder")
		os.Exit(1)
	}

	metricsH := runtimeCtrl.NewMetrics(mgr, metrics.MustMakeRecorder(), infrav1.TerraformFinalizer)

	signalHandlerContext := ctrl.SetupSignalHandler()

	certsReady := make(chan struct{})
	rotator := &mtls.CertRotator{
		Ready:                         certsReady,
		CAName:                        "tf-controller",
		CAOrganization:                "weaveworks",
		DNSName:                       "tf-controller",
		CAValidityDuration:            caValidityDuration,
		RotationCheckFrequency:        rotationCheckFrequency,
		LookaheadInterval:             4 * rotationCheckFrequency, // we do 4 rotation checks ahead
		TriggerCARotation:             make(chan mtls.Trigger),
		TriggerNamespaceTLSGeneration: make(chan mtls.Trigger),
		ClusterDomain:                 clusterDomain,
		UsePodSubdomainResolution:     usePodSubdomainResolution,
	}

	const localHost = "localhost"
	if os.Getenv("INSECURE_LOCAL_RUNNER") == "1" {
		rotator.CAName = localHost
		rotator.CAOrganization = localHost
		rotator.DNSName = localHost
	}

	if err := mtls.AddRotator(signalHandlerContext, mgr, rotator); err != nil {
		setupLog.Error(err, "unable to set up cert rotation")
		os.Exit(1)
	}

	// Cross-namespace refs enabled:
	//
	// --allow... \ --no... | true | false |  - |
	// ---------------------|------|-------|----|
	//     true             |  t   |   t   |  t |
	//     false            |  f   |   f   |  f |
	//     -                |  f   |   t*  |  f |
	//
	// '-' means "not supplied"
	// * is the only place the value of `--no-cross-namespace-refs` is used, so check for this case.
	if !flag.CommandLine.Changed("allow-cross-namespace-refs") && flag.CommandLine.Changed("no-cross-namespace-refs") {
		allowCrossNamespaceRefs = !aclOptions.NoCrossNamespaceRefs
	}

	reconciler := &controllers.TerraformReconciler{
		Client:                    mgr.GetClient(),
		Scheme:                    mgr.GetScheme(),
		EventRecorder:             eventRecorder,
		Metrics:                   metricsH,
		StatusPoller:              polling.NewStatusPoller(mgr.GetClient(), mgr.GetRESTMapper(), polling.Options{}),
		CertRotator:               rotator,
		RunnerGRPCPort:            runnerGRPCPort,
		RunnerCreationTimeout:     runnerCreationTimeout,
		RunnerGRPCMaxMessageSize:  runnerGRPCMaxMessageSize,
		AllowBreakTheGlass:        allowBreakTheGlass,
		ClusterDomain:             clusterDomain,
		NoCrossNamespaceRefs:      !allowCrossNamespaceRefs,
		UsePodSubdomainResolution: usePodSubdomainResolution,
	}

	if err = reconciler.SetupWithManager(mgr, concurrent, httpRetry); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Terraform")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if os.Getenv("INSECURE_LOCAL_RUNNER") == "1" {
		runnerServer := &runner.TerraformRunnerServer{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}
		go func() {
			err := mtls.StartGRPCServerForTesting(runnerServer, "flux-system", "localhost:30000", mgr, rotator)
			if err != nil {
				setupLog.Error(err, "unable to start runner server")
				os.Exit(1)
			}
		}()
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("Starting manager", "version", BuildVersion, "sha", BuildSHA)

	if err := mgr.Start(signalHandlerContext); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
