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
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	infrav1 "github.com/chanwit/tf-controller/api/v1alpha1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var server *ghttp.Server

const (
	timeout  = time.Second * 10
	interval = time.Millisecond * 500
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	// junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}},
	)
}

var (
	ctx    context.Context
	cancel context.CancelFunc
)

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = sourcev1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = infrav1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	By("setting up a http server to mock the source controller's behaviour")
	server = ghttp.NewUnstartedServer()

	By("defining a URL for the TF hello world BLOB to be used as a Source Controller's artifact")
	server.RouteToHandler("GET", "/file.tar.gz", func(writer http.ResponseWriter, request *http.Request) {
		http.ServeFile(writer, request, "data/terraform-hello-world-example.tar.gz")
	})
	By("defining a URL for the TF hello vars BLOB to be used as a Source Controller's artifact")
	server.RouteToHandler("GET", "/env.tar.gz", func(writer http.ResponseWriter, request *http.Request) {
		http.ServeFile(writer, request, "data/terraform-hello-env.tar.gz")
	})
	server.Start()

	By("preparing flux-system namespace")
	fluxSystemNS := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "flux-system",
		},
		Spec: corev1.NamespaceSpec{},
	}
	Expect(k8sClient.Create(ctx, fluxSystemNS)).Should(Succeed())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	err = (&TerraformReconciler{
		Client: k8sManager.GetClient(),
		Scheme: k8sManager.GetScheme(),
	}).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()
}, 60)

var _ = AfterSuite(func() {
	cancel()

	server.Close()

	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

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
