package controllers

import (
	"sync/atomic"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGracefulShutdownComplete(t *testing.T) {
	g := NewWithT(t)

	reconciler := &TerraformReconciler{
		ShutdownTimeout: 30 * time.Second,
	}

	// Create a couple active reconciliations for shutdown to wait
	reconciler.activeReconciliations.Add(2)

	done := atomic.Bool{}
	go func() {
		reconciler.ShutdownCoordinator(t.Context())
		done.Store(true)
	}()

	g.Eventually(func() bool {
		return reconciler.shutdownStarted.Load()
	}).WithTimeout(3 * time.Second).WithPolling(100 * time.Millisecond).Should(BeTrue())

	g.Expect(done.Load()).To(BeFalse())

	// 1 reconciliation have completed, 1 remaining
	reconciler.activeReconciliations.Done()

	time.Sleep(1 * time.Second)
	g.Expect(done.Load()).To(BeFalse())

	// All reconciliations have completed
	reconciler.activeReconciliations.Done()

	// Confirm that ShutdownCoordinator() completed execution
	g.Eventually(func() bool {
		return done.Load()
	}).WithTimeout(3 * time.Second).WithPolling(100 * time.Millisecond).Should(BeTrue())
}

func TestGracefulShutdownTimeout(t *testing.T) {
	g := NewWithT(t)

	reconciler := &TerraformReconciler{
		ShutdownTimeout: 1 * time.Second,
	}

	reconciler.activeReconciliations.Add(1)

	done := atomic.Bool{}
	go func() {
		reconciler.ShutdownCoordinator(t.Context())
		done.Store(true)
	}()

	g.Eventually(func() bool {
		return reconciler.shutdownStarted.Load()
	}).WithTimeout(3 * time.Second).WithPolling(100 * time.Millisecond).Should(BeTrue())

	// Confirm that ShutdownCoordinator() exited due to timeout despite active reconciliations
	g.Eventually(func() bool {
		return done.Load()
	}).WithTimeout(3 * time.Second).WithPolling(100 * time.Millisecond).Should(BeTrue())
}

func TestReconcileRejectedAfterShutdown(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	reconciler := &TerraformReconciler{
		Client:          fake.NewClientBuilder().WithScheme(scheme).Build(),
		EventRecorder:   &record.FakeRecorder{},
		ShutdownTimeout: 5 * time.Second,
	}

	reconciler.shutdownStarted.Store(true)

	req := ctrl.Request{}

	result, err := reconciler.Reconcile(t.Context(), req)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))
}
