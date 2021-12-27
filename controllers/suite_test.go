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

package controllers

import (
	"context"
	"fmt"
	"github.com/onsi/gomega/ghttp"
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	"strings"
	"testing"
	"time"

	infrav1 "github.com/chanwit/tf-controller/api/v1alpha1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"
	"k8s.io/client-go/kubernetes/scheme"
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
	timeout  = time.Second * 10
	interval = time.Millisecond * 500
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	server    *ghttp.Server
)

var (
	ctx    context.Context
	cancel context.CancelFunc
)

func TestMain(m *testing.M) {
	var err error

	logf.SetLogger(zap.New(zap.WriteTo(os.Stderr), zap.UseDevMode(false)))
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

	err = sourcev1.AddToScheme(scheme.Scheme)
	if err != nil {
		panic(err.Error())
	}

	err = infrav1.AddToScheme(scheme.Scheme)
	if err != nil {
		panic(err.Error())
	}

	//+kubebuilder:scaffold:scheme
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
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
	server.RouteToHandler("GET", "/2222.tar.gz", func(writer http.ResponseWriter, request *http.Request) {
		http.ServeFile(writer, request, "data/terraform-hello-world-example-2.tar.gz")
	})
	server.RouteToHandler("GET", "/bad.tar.gz", func(writer http.ResponseWriter, request *http.Request) {
		http.ServeFile(writer, request, "data/bad.tar.gz")
	})

	// "defining a URL for the TF hello vars BLOB to be used as a Source Controller's artifact"
	server.RouteToHandler("GET", "/env.tar.gz", func(writer http.ResponseWriter, request *http.Request) {
		http.ServeFile(writer, request, "data/terraform-hello-env.tar.gz")
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
		Scheme: scheme.Scheme,
	})
	if err != nil {
		panic(err.Error())
	}

	err = (&TerraformReconciler{
		Client:        k8sManager.GetClient(),
		Scheme:        k8sManager.GetScheme(),
		EventRecorder: k8sManager.GetEventRecorderFor("tf-controller"),
		StatusPoller:  polling.NewStatusPoller(k8sManager.GetClient(), k8sManager.GetRESTMapper()),
	}).SetupWithManager(k8sManager)
	if err != nil {
		panic(err.Error())
	}

	go func() {
		err = k8sManager.Start(ctx)
		if err != nil {
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

func spec(text string) {
	preamble := "\x1b[1mSPEC\x1b[0m"
	fmt.Fprintln(os.Stderr, preamble+": "+text)
}

func given(text string) {
	preamble := "\x1b[1mGIVEN\x1b[0m"
	fmt.Fprintln(os.Stderr, preamble+": "+text)
}

func when(text string) {
	preamble := "\x1b[1mWHEN\x1b[0m"
	fmt.Fprintln(os.Stderr, preamble+": "+text)
}

func it(text string) {
	preamble := "\x1b[1mIT\x1b[0m"
	fmt.Fprintln(os.Stderr, preamble+" "+text)
}

func by(text string) {
	preamble := "\x1b[1mBY\x1b[0m"
	fmt.Fprintln(os.Stderr, preamble+": "+text)
}
