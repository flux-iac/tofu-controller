package controllers

import (
	"testing"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	. "github.com/onsi/gomega"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_externalartifact_source_requires_feature_gate(t *testing.T) {
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

	// Client is nil: with the gate off, resolveSource must not look the ExternalArtifact up.
	reconciler := &TerraformReconciler{EnableExternalArtifact: false}
	_, err := reconciler.resolveSource(t.Context(), terraform)
	g.Expect(err).To(HaveOccurred())
}
