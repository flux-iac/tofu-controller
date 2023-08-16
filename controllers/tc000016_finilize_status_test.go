package controllers

import (
	"context"
	"testing"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	. "github.com/onsi/gomega"

	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func Test_000016_finalize_status(t *testing.T) {
	Spec("This spec describes the behaviour of the Terraform controller when finalizing the status of a Terraform resource")
	It("should set the observedGeneration and LastHandledReconcileAt after reconcile")

	const (
		sourceName    = "test-finalize-status"
		terraformName = "tf-cross-ns"
	)
	g := NewWithT(t)
	ctx := context.Background()

	Given("a GitRepository")
	By("defining a new GitRepository resource.")
	updatedTime := time.Now()
	testRepo := sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sourceName,
			Namespace: "flux-system",
		},
		Spec: sourcev1.GitRepositorySpec{
			URL: "https://github.com/openshift-fluxv2-poc/podinfo",
			Reference: &sourcev1.GitRepositoryRef{
				Branch: "main",
			},
			Interval: metav1.Duration{Duration: time.Second * 30},
		},
	}

	By("creating the GitRepository resource in the cluster.")
	It("should be created successfully.")
	g.Expect(k8sClient.Create(ctx, &testRepo)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &testRepo)).Should(Succeed()) }()

	Given("the GitRepository's reconciled status")
	By("setting the GitRepository's status, with the downloadable BLOB's URL, and the correct checksum.")
	testRepo.Status = sourcev1.GitRepositoryStatus{
		ObservedGeneration: int64(1),
		Conditions: []metav1.Condition{
			{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Time{Time: updatedTime},
				Reason:             "GitOperationSucceed",
				Message:            "Fetched revision: main/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			},
		},
		Artifact: &sourcev1.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/file.tar.gz",
			Revision:       "main/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Digest:         "sha256:80ddfd18eb96f7d31cadc1a8a5171c6e2d95df3f6c23b0ed9cd8dddf6dba1406",
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}

	It("should be updated successfully.")
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	Given("a Terraform resource")
	By("creating a new TF resource.")
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "default",
			Annotations: map[string]string{
				meta.ReconcileRequestAnnotation: updatedTime.String(),
			},
		},
		Spec: infrav1.TerraformSpec{
			Path: "./terraform-hello-world-example",
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
	t.Cleanup(func() { g.Expect(k8sClient.Delete(ctx, &helloWorldTF)).Should(Succeed()) })

	It("should have observedGeneration and LastHandledReconcileAt set")
	helloWorldTFKey := client.ObjectKeyFromObject(&helloWorldTF)
	g.Eventually(func() int64 {
		var createdHelloWorldTF infrav1.Terraform
		g.Expect(k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)).To(Succeed())
		return createdHelloWorldTF.Status.ObservedGeneration
	}, timeout, interval).Should(Equal(int64(1)))

	g.Eventually(func() string {
		var createdHelloWorldTF infrav1.Terraform
		g.Expect(k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)).To(Succeed())
		return createdHelloWorldTF.Status.LastHandledReconcileAt
	}, timeout, interval).Should(Equal(updatedTime.String()))
}
