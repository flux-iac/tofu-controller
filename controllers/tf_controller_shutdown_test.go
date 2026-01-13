package controllers

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

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

	result, err := reconciler.Reconcile(context.Background(), req)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))
}
