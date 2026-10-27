package controllers

import (
	"context"
	"errors"
	"testing"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// TestCrossNamespaceDependsOnIsTerminal checks a cross-namespace dependsOn
// denial is handled like a cross-namespace source denial: Stalled and
// Ready False AccessDenied, a Warning event, and a terminal error.
func TestCrossNamespaceDependsOnIsTerminal(t *testing.T) {
	g := NewWithT(t)
	src := helloWorldSource(newArtifactServer(t))
	obj := fakeTerraform("depends-on-denied")
	obj.Spec.DependsOn = []meta.NamespacedObjectReference{{Name: "dep", Namespace: "other"}}
	conditions.MarkReconciling(obj, meta.ProgressingReason, "Fulfilling prerequisites")

	c, patcher := newFakePatcher(t, obj, src)
	r := newFakeReconciler(c)
	r.NoCrossNamespaceRefs = true

	err := r.checkDependencies(context.Background(), obj, src)
	g.Expect(err).To(HaveOccurred())
	res, err := r.dependenciesNotReady(context.Background(), patcher, obj, src.GetArtifact().Revision, err)

	g.Expect(res).To(Equal(reconcile.Result{}))
	g.Expect(err).To(HaveOccurred())
	g.Expect(errors.Is(err, reconcile.TerminalError(nil))).To(BeTrue(), "error should be terminal")

	g.Expect(conditions.IsTrue(obj, meta.StalledCondition)).To(BeTrue())
	g.Expect(conditions.GetReason(obj, meta.StalledCondition)).To(Equal(infrav1.AccessDeniedReason))
	g.Expect(conditions.IsFalse(obj, meta.ReadyCondition)).To(BeTrue())
	g.Expect(conditions.GetReason(obj, meta.ReadyCondition)).To(Equal(infrav1.AccessDeniedReason))
	g.Expect(conditions.Has(obj, meta.ReconcilingCondition)).To(BeFalse())

	recorded := drainEvents(r.EventRecorder.(*events.FakeRecorder))
	g.Expect(recorded).To(HaveLen(1))
	g.Expect(recorded[0]).To(ContainSubstring(corev1.EventTypeWarning))
	g.Expect(recorded[0]).To(ContainSubstring(infrav1.AccessDeniedReason))
}
