package controllers

import (
	"context"
	"testing"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000190_unsupported_source_should_error(t *testing.T) {
	Spec("This spec describes the behaviour when an unsupported source reference is provided.")

	const (
		sourceName    = "unsupported"
		terraformName = "test-unsupported-source"
	)

	g := NewWithT(t)

	ctx := context.Background()

	By("submitting a new TF with an unsupported source")
	tfResource := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "HelmRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
		},
	}

	It("should return an error")
	_, err := reconciler.getSource(ctx, tfResource)
	g.Expect(err).ShouldNot(BeNil())
}
