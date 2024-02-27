package controllers

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Test_000015_cross_namespace_cliconfigsecretref_denied_test(t *testing.T) {
	Spec("This spec describes the behaviour of a Terraform resource when .spec.cliConfigSecretRef is in another namespace and cross-namespace refs are not allowed.")
	It("should be reconciled to have an access denied error.")

	By("setting the reconciler to disallow cross-namespace refs")
	defer func(original bool) {
		reconciler.NoCrossNamespaceRefs = original
	}(reconciler.NoCrossNamespaceRefs)
	reconciler.NoCrossNamespaceRefs = true

	const (
		sourceName    = "gr-source"
		terraformName = "tf-cross-ns-cliconfigsecret"
	)
	g := NewWithT(t)
	ctx := context.Background()

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
	t.Cleanup(func() { g.Expect(k8sClient.Delete(ctx, &testRepo)).Should(Succeed()) })

	Given("the GitRepository's reconciled status.")
	By("setting the GitRepository's status, with the downloadable BLOB's URL, and the correct checksum.")
	updatedTime := time.Now()
	testRepo.Status = sourcev1.GitRepositoryStatus{
		ObservedGeneration: int64(1),
		Conditions: []metav1.Condition{
			{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Time{Time: updatedTime},
				Reason:             "GitOperationSucceed",
				Message:            "Fetched revision: master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			},
		},
		Artifact: &sourcev1.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/file.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Digest:         "sha256:80ddfd18eb96f7d31cadc1a8a5171c6e2d95df3f6c23b0ed9cd8dddf6dba1406",
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}

	It("should be updated successfully.")
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	Given("a Terraform resource")
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
				Kind: "GitRepository",
				Name: testRepo.Name,
			},
			Interval: metav1.Duration{Duration: time.Second * 10},
			CliConfigSecretRef: &corev1.SecretReference{
				Name: "gr-source", Namespace: "other-ns",
			},
		},
	}
	It("should be created and attached successfully.")
	g.Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())
	t.Cleanup(func() { g.Expect(k8sClient.Delete(ctx, &helloWorldTF)).Should(Succeed()) })

	It("should be access denied error.")
	By("checking that the Ready's reason of the TF resource become `AccessDenied`.")

	helloWorldTFKey := client.ObjectKeyFromObject(&helloWorldTF)
	var readyCondition *metav1.Condition
	g.Eventually(func() interface{} {
		var createdHelloWorldTF infrav1.Terraform
		g.Expect(k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)).To(Succeed())
		conditions := createdHelloWorldTF.Status.Conditions
		readyCondition = meta.FindStatusCondition(conditions, "Ready")
		// it's possible to see "Ready=Progressing" here, before it fails.
		return readyCondition != nil && readyCondition.Reason != "Progressing"
	}, timeout, interval).Should(BeTrue())

	g.Expect(*readyCondition).To(
		gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"Type":    Equal("Ready"),
			"Reason":  Equal(infrav1.AccessDeniedReason),
			"Message": Equal("cannot access secret other-ns/gr-source, cross-namespace references have been disabled"),
		}))
}
