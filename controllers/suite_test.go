/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required By applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/weaveworks/tf-controller/mtls"
	"github.com/weaveworks/tf-controller/runner"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"github.com/onsi/gomega/ghttp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"

	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	//+kubebuilder:scaffold:imports
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

const (
	// longer timeout duration is helpful to avoid flakiness when
	// asserting on k8s resources created via Terraform
	timeout  = time.Second * 30
	interval = time.Millisecond * 500
)

var (
	cfg          *rest.Config
	k8sClient    client.Client
	testEnv      *envtest.Environment
	server       *ghttp.Server
	rotator      *mtls.CertRotator
	reconciler   *TerraformReconciler
	runnerServer *runner.TerraformRunnerServer
)

var (
	ctx    context.Context
	cancel context.CancelFunc
)

func TestMain(m *testing.M) {
	var err error
	var logSink io.Writer

	logSink = os.Stderr

	if os.Getenv("DISABLE_K8S_LOGS") == "1" {
		logSink = io.Discard
	}

	logf.SetLogger(zap.New(zap.WriteTo(logSink), zap.UseDevMode(false)))

	ctx, cancel = context.WithCancel(context.TODO())
	// "bootstrapping test environment"
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	if err != nil {
		panic(err.Error())
	}
	if cfg == nil {
		panic("cfg cannot be nil")
	}

	scheme := runtime.NewScheme()

	err = clientgoscheme.AddToScheme(scheme)
	if err != nil {
		panic(err.Error())
	}

	err = sourcev1.AddToScheme(scheme)
	if err != nil {
		panic(err.Error())
	}

	err = infrav1.AddToScheme(scheme)
	if err != nil {
		panic(err.Error())
	}

	//+kubebuilder:scaffold:scheme
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		panic(err.Error())
	}
	if k8sClient == nil {
		panic("k8sClient cannot be nil")
	}

	// "setting up a http server to mock the source controller's behaviour"
	server = ghttp.NewUnstartedServer()

	// "defining a URL for the TF hello world BLOB to be used as a Source Controller's artifact"
	server.RouteToHandler("GET", "/file.tar.gz", func(writer http.ResponseWriter, request *http.Request) {
		http.ServeFile(writer, request, "data/terraform-hello-world-example.tar.gz")
	})
	server.RouteToHandler("GET", "/terraform-outputs-dots.tar.gz", func(writer http.ResponseWriter, request *http.Request) {
		http.ServeFile(writer, request, "data/terraform-outputs-dots.tar.gz")
	})
	server.RouteToHandler("GET", "/2222.tar.gz", func(writer http.ResponseWriter, request *http.Request) {
		http.ServeFile(writer, request, "data/terraform-hello-world-example-2.tar.gz")
	})
	server.RouteToHandler("GET", "/bad.tar.gz", func(writer http.ResponseWriter, request *http.Request) {
		http.ServeFile(writer, request, "data/bad.tar.gz")
	})
	server.RouteToHandler("GET", "/env.tar.gz", func(writer http.ResponseWriter, request *http.Request) {
		http.ServeFile(writer, request, "data/terraform-hello-env.tar.gz")
	})
	server.RouteToHandler("GET", "/tf-k8s-configmap.tar.gz", func(writer http.ResponseWriter, request *http.Request) {
		http.ServeFile(writer, request, "data/tf-k8s-configmap.tar.gz")
	})
	server.RouteToHandler("GET", "/tf-k8s-configmap-unrelated-change.tar.gz", func(writer http.ResponseWriter, request *http.Request) {
		http.ServeFile(writer, request, "data/tf-k8s-configmap-unrelated-change.tar.gz")
	})
	server.RouteToHandler("GET", "/tfc-helloworld.tar.gz", func(writer http.ResponseWriter, request *http.Request) {
		http.ServeFile(writer, request, "data/tfc-helloworld.tar.gz")
	})
	server.RouteToHandler("GET", "/tf-multi-var.tar.gz", func(writer http.ResponseWriter, request *http.Request) {
		http.ServeFile(writer, request, "data/tf-multi-var-with-outputs.tar.gz")
	})
	server.RouteToHandler("GET", "/tf-health-check.tar.gz", func(writer http.ResponseWriter, request *http.Request) {
		http.ServeFile(writer, request, "data/tf-health-check-example.tar.gz")
	})
	server.RouteToHandler("GET", "/tf-hcl-var-with-outputs.tar.gz", func(writer http.ResponseWriter, request *http.Request) {
		http.ServeFile(writer, request, "data/tf-hcl-var-with-outputs.tar.gz")
	})
	// for health check http test
	server.RouteToHandler("GET", "/get", func(writer http.ResponseWriter, request *http.Request) {
		ghttp.RespondWith(http.StatusOK, "ok")
	})

	server.Start()

	// "preparing flux-system namespace"
	fluxSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "flux-system",
		},
		Spec: corev1.NamespaceSpec{},
	}
	err = k8sClient.Create(ctx, fluxSystemNS)
	if err != nil {
		panic(err.Error())
	}

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		panic(err.Error())
	}

	certsReady := make(chan struct{})

	rotator = &mtls.CertRotator{
		CAName:         "localhost",
		CAOrganization: "localhost",
		DNSName:        "localhost",
		SecretKey: types.NamespacedName{
			Namespace: "flux-system",
			Name:      "tf-controller.tls",
		},
		Ready:                  certsReady,
		CAValidityDuration:     time.Hour * 24 * 7,
		CertValidityDuration:   5*time.Minute + 1*time.Hour, // since the cert lookaheadInterval is set to 1 hour, using 1hr5m so that the cert is valid for 5 mins
		RotationCheckFrequency: 10 * time.Second,
		LookaheadInterval:      1 * time.Hour,
	}

	if err := mtls.AddRotator(ctx, k8sManager, rotator); err != nil {
		panic(err)
	}

	reconciler = &TerraformReconciler{
		Client:         k8sManager.GetClient(),
		Scheme:         k8sManager.GetScheme(),
		EventRecorder:  k8sManager.GetEventRecorderFor("tf-controller"),
		StatusPoller:   polling.NewStatusPoller(k8sManager.GetClient(), k8sManager.GetRESTMapper(), polling.Options{}),
		CertRotator:    rotator,
		RunnerGRPCPort: 30000,
	}

	// We use 1 concurrent and 10s httpRetry in the test
	err = reconciler.SetupWithManager(k8sManager, 1, 10)
	if err != nil {
		panic(err.Error())
	}

	runnerServer = &runner.TerraformRunnerServer{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
	}

	go mtls.StartGRPCServerForTesting(ctx, runnerServer, "flux-system", "localhost:30000", k8sManager, rotator)

	go func() {
		if err := k8sManager.Start(ctx); err != nil {
			panic(err.Error())
		}
	}()

	code := m.Run()
	cancel()
	server.Close()

	// "tearing down the test environment"
	err = testEnv.Stop()
	if err != nil {
		panic(err.Error())
	}

	os.Exit(code)
}

func findKubeConfig(e *envtest.Environment) (string, error) {
	files, err := ioutil.ReadDir(e.ControlPlane.APIServer.CertDir)
	if err != nil {
		return "", err
	}
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".kubecfg") {
			return filepath.Join(e.ControlPlane.APIServer.CertDir, file.Name()), nil
		}
	}

	return "", fmt.Errorf("file not found")
}

func Spec(text string) {
	preamble := "\x1b[1mSPEC\x1b[0m"
	fmt.Fprintln(os.Stderr, preamble+": "+text)
}

func Given(text string) {
	preamble := "\x1b[1mGIVEN\x1b[0m"
	fmt.Fprintln(os.Stderr, preamble+": "+text)
}

func when(text string) {
	preamble := "\x1b[1mWHEN\x1b[0m"
	fmt.Fprintln(os.Stderr, preamble+": "+text)
}

func It(text string) {
	preamble := "\x1b[1mIT\x1b[0m"
	fmt.Fprintln(os.Stderr, preamble+" "+text)
}

func By(text string) {
	preamble := "\x1b[1mBY\x1b[0m"
	fmt.Fprintln(os.Stderr, preamble+": "+text)
}
