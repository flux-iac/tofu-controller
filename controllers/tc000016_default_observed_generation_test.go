package controllers

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Test_000016_default_observed_generation(t *testing.T) {
	Spec("This spec describes default value of a Terraform resource")
	It("should set the observedGeneration to -1 when the resource is created")

	var (
		terraformName = "tf-" + rand.String(6)
	)
	g := NewWithT(t)
	ctx := context.Background()

	Given("a Terraform resource")
	By("creating a new TF resource.")
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "default",
		},
		Spec: infrav1.TerraformSpec{
			Path: "./terraform-hello-world-example",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      "foo",
				Namespace: "flux-system",
			},
			Interval: metav1.Duration{Duration: time.Second * 10},
		},
	}
	It("should be created and attached successfully.")
	g.Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())
	t.Cleanup(func() { g.Expect(k8sClient.Delete(ctx, &helloWorldTF)).Should(Succeed()) })

	It("should have observedGeneration set to -1")
	helloWorldTFKey := client.ObjectKeyFromObject(&helloWorldTF)
	g.Eventually(func() int64 {
		var createdHelloWorldTF infrav1.Terraform
		g.Expect(k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)).To(Succeed())
		return createdHelloWorldTF.Status.ObservedGeneration
	}, timeout, interval).Should(Equal(int64(-1)))
}
