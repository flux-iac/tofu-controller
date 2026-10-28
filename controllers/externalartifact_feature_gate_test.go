package controllers

import (
	"errors"
	"testing"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	. "github.com/onsi/gomega"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// With the ExternalArtifact gate off, a Terraform that names one as its source
// is stalled for good, not retried: only a restart with the gate on, or a new
// sourceRef, can help.
func Test_externalartifact_source_without_feature_gate_is_stalled(t *testing.T) {
	g := NewWithT(t)

	terraform := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{Name: "gate-test", Namespace: "flux-system"},
		Spec: infrav1.TerraformSpec{
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind: sourcev1.ExternalArtifactKind,
				Name: "does-not-exist",
			},
		},
	}
	c, _ := newFakePatcher(t, terraform)
	reconciler := newFakeReconciler(c)
	reconciler.EnableExternalArtifact = false

	_, ok, _, err := reconciler.resolveArtifact(t.Context(), terraform)

	g.Expect(ok).To(BeFalse())
	g.Expect(errors.Is(err, reconcile.TerminalError(nil))).To(BeTrue())
	g.Expect(conditions.IsTrue(terraform, meta.StalledCondition)).To(BeTrue())
	g.Expect(conditions.GetReason(terraform, meta.ReadyCondition)).To(Equal(infrav1.AccessDeniedReason))
}
