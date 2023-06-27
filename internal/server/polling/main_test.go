package polling

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	k8sClient client.Client
)

// TestMain wraps all the other tests in this file by starting an
// testEnv (Kubernetes API), and stopping it after the tests.
func TestMain(m *testing.M) {
	testEnv := &envtest.Environment{}
	cfg, err := testEnv.Start()
	if err != nil {
		panic(err)
	}
	defer testEnv.Stop()

	c, err := client.New(cfg, client.Options{})
	if err != nil {
		panic(err)
	}
	k8sClient = c

	m.Run()
}

var (
	nscounter atomic.Int32
)

// newNamespace creates a namespace in which to run a test. In
// general, an individual test will want to create a namespace at its
// start, use its name for any other objects it creates, and delete
// the namespace afterward. Test_scaffold shows how to do this.
func newNamespace(g Gomega) *corev1.Namespace {
	num := nscounter.Add(1)
	name := fmt.Sprintf("test-ns-%d", num)
	ns := corev1.Namespace{}
	ns.SetName(name)
	g.ExpectWithOffset(1, k8sClient.Create(context.TODO(), &ns)).To(Succeed())
	return &ns
}

// Minimal test to check the scaffolding works.
func Test_scaffold(t *testing.T) {
	g := NewWithT(t)
	ns := newNamespace(g)
	// here is where you'd create some objects in the namespace, as
	// part of your test case.
	t.Cleanup(func() { g.Expect(k8sClient.Delete(context.TODO(), ns)).To(Succeed()) })
}
