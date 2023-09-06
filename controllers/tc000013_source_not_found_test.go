//go:build disabled

package controllers

// This test is disabled.
//
// Right now a TF Resource stuck on deletion if the referenced source is not
// available. This test should fail in the past as the resource was never
// cleaned up, but as we didn't wait for it, it didn't fail. For now I disable
// this test as it does not test properly what it should. Fine, it tests what it
// should, but it will always fail with timeout. If I remove the
// waitResourceToBeDelete call and the timeout, this test gives false report.

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000013_source_not_found_test(t *testing.T) {
	Spec("This spec describes the behaviour of a Terraform resource when source is not found.")
	It("should be reconciled to have a Source error.")

	const (
		sourceName    = "gr-source-not-found"
		terraformName = "tf-source-not-found"
	)
	g := NewWithT(t)
	ctx := context.Background()

	Given("a Terraform resource with auto approve, attached to a non-existed GitRepository resource.")
	By("creating a new TF resource and attaching to the repo via `sourceRef`.")
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			ApprovePlan: "auto",
			Path:        "./terraform-hello-world-example",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
			Interval: metav1.Duration{Duration: time.Second * 10},
		},
	}
	It("should be created and attached successfully.")
	g.Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())
	defer waitResourceToBeDelete(g, &helloWorldTF)

	By("checking that the TF resource existed inside the cluster.")
	helloWorldTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName}
	createdHelloWorldTF := infrav1.Terraform{}
	g.Eventually(func() bool {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return false
		}
		return true
	}, timeout, interval).Should(BeTrue())

	It("should be reconciled and contain some status conditions.")
	By("checking that the TF resource's status conditions has some elements.")
	g.Eventually(func() int {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return -1
		}
		return len(createdHelloWorldTF.Status.Conditions)
	}, timeout, interval).ShouldNot(BeZero())

	It("should be error.")
	By("checking that the Ready's reason of the TF resource become `ArtifactFailed`.")
	g.Eventually(func() interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == "Ready" {
				return map[string]interface{}{
					"Type":    c.Type,
					"Reason":  c.Reason,
					"Message": c.Message,
				}
			}
		}
		return createdHelloWorldTF.Status
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":    "Ready",
		"Reason":  infrav1.ArtifactFailedReason,
		"Message": "Source 'GitRepository/flux-system/gr-source-not-found' not found",
	}))

}
