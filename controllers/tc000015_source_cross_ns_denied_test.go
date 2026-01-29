package controllers

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Test_000015_cross_namespace_source_denied_test(t *testing.T) {
	Spec("This spec describes the behaviour of a Terraform resource when source is in another namespace and cross-namespace refs are not allowed.")
	It("should be reconciled to have a Source error.")

	By("setting the reconciler to disallow cross-namespace refs")
	defer func(original bool) {
		reconciler.NoCrossNamespaceRefs = original
	}(reconciler.NoCrossNamespaceRefs)
	reconciler.NoCrossNamespaceRefs = true

	const (
		sourceName    = "gr-source"
		terraformName = "tf-cross-ns"
	)
	g := NewWithT(t)
	ctx := t.Context()

	Given("a GitRepository")
	By("defining a new GitRepository resource.")
	testRepo := sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sourceName,
			Namespace: "flux-system",
		},
		Spec: sourcev1.GitRepositorySpec{
			URL: "https://github.com/openshift-fluxv2-poc/podinfo",
			Reference: &sourcev1.GitRepositoryRef{
				Branch: "master",
			},
			Interval: metav1.Duration{Duration: time.Second * 30},
		},
	}
	By("creating the GitRepository resource in the cluster.")
	It("should be created successfully.")
	g.Expect(k8sClient.Create(ctx, &testRepo)).Should(Succeed())
	t.Cleanup(func() { g.Expect(k8sClient.Delete(context.Background(), &testRepo)).Should(Succeed()) })

	Given("a Terraform resource, attached to a GitRepository resource in another namespace.")
	By("creating a new TF resource and attaching to the repo via `sourceRef`.")
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "default",
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
	t.Cleanup(func() { g.Expect(k8sClient.Delete(context.Background(), &helloWorldTF)).Should(Succeed()) })

	It("should be access denied error.")
	By("checking that the Ready's reason of the TF resource become `AccessDenied`.")

	helloWorldTFKey := client.ObjectKeyFromObject(&helloWorldTF)
	var readyCondition *metav1.Condition
	g.Eventually(func() any {
		var createdHelloWorldTF infrav1.Terraform
		g.Expect(k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)).To(Succeed())
		conditions := createdHelloWorldTF.Status.Conditions
		readyCondition = meta.FindStatusCondition(conditions, "Ready")
		return readyCondition
	}, timeout, interval).ShouldNot(BeNil())

	g.Expect(*readyCondition).To(
		gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"Type":    Equal("Ready"),
			"Reason":  Equal(infrav1.AccessDeniedReason),
			"Message": Equal("cannot access GitRepository/flux-system/gr-source, cross-namespace references have been disabled"),
		}))
}
