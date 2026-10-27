package controllers

import (
	"context"
	"errors"
	"testing"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/apis/meta"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestHealthCheckFailureSetsReadyFalse checks a failed health check leaves
// Ready False HealthChecksFailed with the check error as the message, next to
// the HealthCheck condition.
func TestHealthCheckFailureSetsReadyFalse(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	r := newFakeReconciler(fake.NewClientBuilder().WithScheme(scheme).Build())

	obj := fakeTerraform("health-fail")
	obj.Spec.WriteOutputsToSecret = &infrav1.WriteOutputsToSecretSpec{Name: "out"}
	obj.Spec.HealthChecks = []infrav1.HealthCheck{{Name: "tcp", Type: infrav1.HealthCheckTypeTCP, Address: "127.0.0.1:1"}}
	fr := newFakeRunner()
	fr.errs["GetOutputs"] = errors.New("boom")
	s := &runnerSession{client: fr, revision: "rev"}

	err := r.doHealthChecks(context.Background(), s, obj)
	g.Expect(err).To(HaveOccurred())

	var ready, hc *metav1.Condition
	for i := range obj.Status.Conditions {
		switch obj.Status.Conditions[i].Type {
		case meta.ReadyCondition:
			ready = &obj.Status.Conditions[i]
		case infrav1.ConditionTypeHealthCheck:
			hc = &obj.Status.Conditions[i]
		}
	}
	g.Expect(hc).NotTo(BeNil())
	g.Expect(hc.Reason).To(Equal(infrav1.HealthChecksFailedReason))
	g.Expect(ready).NotTo(BeNil())
	g.Expect(ready.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(ready.Reason).To(Equal(infrav1.HealthChecksFailedReason))
	g.Expect(ready.Message).To(Equal(err.Error()))
	g.Expect(hc.Message).To(Equal(err.Error()))
}

// TestHealthCheckFailureKeepsPlanPrompt checks a failed health check on a pass
// that planned but did not apply leaves Ready alone, so it keeps telling the
// user to approve the pending plan; the HealthCheck condition carries the
// failure.
func TestHealthCheckFailureKeepsPlanPrompt(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	r := newFakeReconciler(fake.NewClientBuilder().WithScheme(scheme).Build())

	obj := fakeTerraform("health-fail-pending")
	obj.Spec.WriteOutputsToSecret = &infrav1.WriteOutputsToSecretSpec{Name: "out"}
	obj.Spec.HealthChecks = []infrav1.HealthCheck{{Name: "tcp", Type: infrav1.HealthCheckTypeTCP, Address: "127.0.0.1:1"}}
	infrav1.TerraformPlannedWithChanges(obj, "main@sha1:b8e362c206e3d0cbb7ed22ced771a0056455a2fb", false, "Plan generated")
	prompt := apimeta.FindStatusCondition(obj.Status.Conditions, meta.ReadyCondition).DeepCopy()
	fr := newFakeRunner()
	fr.errs["GetOutputs"] = errors.New("boom")
	s := &runnerSession{client: fr, revision: "rev"}

	err := r.doHealthChecks(context.Background(), s, obj)
	g.Expect(err).To(HaveOccurred())

	g.Expect(apimeta.FindStatusCondition(obj.Status.Conditions, meta.ReadyCondition)).To(Equal(prompt))
	hc := apimeta.FindStatusCondition(obj.Status.Conditions, infrav1.ConditionTypeHealthCheck)
	g.Expect(hc).NotTo(BeNil())
	g.Expect(hc.Reason).To(Equal(infrav1.HealthChecksFailedReason))
}
