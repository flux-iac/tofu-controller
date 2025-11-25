package controllers

import (
	"context"
	"testing"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000191_missing_source_should_error(t *testing.T) {
	Spec("This spec describes the behaviour when a valid source reference cannot be found.")

	const (
		sourceName    = "missing"
		terraformName = "test-missing-source"
	)

	g := NewWithT(t)

	ctx := context.Background()

	By("submitting a Terraform resource referencing a git source which does not exist")
	tfResource := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
		},
	}

	It("should return a NotFound error")
	_, err := reconciler.getSource(ctx, tfResource)
	g.Expect(apierrors.IsNotFound(err)).Should(BeTrue())

	By("submitting a Terraform resource referencing a bucket source which does not exist")
	tfResource = &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "Bucket",
				Name:      sourceName,
				Namespace: "flux-system",
			},
		},
	}

	It("should return a NotFound error")
	_, err = reconciler.getSource(ctx, tfResource)
	g.Expect(apierrors.IsNotFound(err)).Should(BeTrue())
}
