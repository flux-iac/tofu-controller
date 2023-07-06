package bbp

import (
	"context"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"

	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/onsi/gomega"

	sourcev1 "github.com/fluxcd/source-controller/api/v1beta2"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
)

var (
	testEnv   *envtest.Environment
	k8sClient client.Client
	k8sConfig *rest.Config
)

// TestMain wraps all the other tests in this file by starting an
// testEnv (Kubernetes API), and stopping it after the tests.
func TestMain(m *testing.M) {
	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := testEnv.Start()
	if err != nil {
		panic(err)
	}
	defer testEnv.Stop()

	if err = infrav1.AddToScheme(scheme.Scheme); err != nil {
		panic(err)
	}
	if err = sourcev1.AddToScheme(scheme.Scheme); err != nil {
		panic(err)
	}

	k8sConfig = cfg

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		panic(err)
	}

	m.Run()
}

var (
	nscounter atomic.Int32
)

// newNamespace creates a namespace in which to run a test. In
// general, an individual test will want to create a namespace at its
// start, use its name for any other objects it creates, and delete
// the namespace afterward. Test_scaffold shows how to do this.
func newNamespace(g gomega.Gomega) *corev1.Namespace {
	num := nscounter.Add(1)
	name := fmt.Sprintf("test-ns-%d", num)
	ns := corev1.Namespace{}
	ns.SetName(name)
	g.ExpectWithOffset(1, k8sClient.Create(context.TODO(), &ns)).To(gomega.Succeed())
	return &ns
}

func expectToSucceed(g gomega.Gomega, arg interface{}) {
	g.ExpectWithOffset(1, arg).To(gomega.Succeed())
}

// Minimal test to check the scaffolding works.
func Test_scaffold(t *testing.T) {
	g := gomega.NewWithT(t)
	ns := newNamespace(g)
	// here is where you'd create some objects in the namespace, as
	// part of your test case.
	t.Cleanup(func() { expectToSucceed(g, k8sClient.Delete(context.TODO(), ns)) })
}
