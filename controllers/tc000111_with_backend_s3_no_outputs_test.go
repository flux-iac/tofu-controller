package controllers

import (
	"context"
	"fmt"
	"testing"
	"time"

	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"
	. "github.com/onsi/gomega"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000111_with_backend_s3_no_outputs_test(t *testing.T) {
	Spec("This spec describes the behaviour of a Terraform resource with backend configured, and `auto` approve.")
	It("should be reconciled from the plan state, to the apply state and have a correct TFSTATE stored inside the cluster as a Secret.")

	const (
		sourceName    = "test-tf-with-backend-no-output"
		terraformName = "helloworld-with-backend-no-outputs"
	)

	g := NewWithT(t)
	ctx := context.Background()

	Given("a GitRepository")
	By("defining a new Git repository resource.")
	updatedTime := time.Now()
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
			Interval:          metav1.Duration{Duration: time.Second * 30},
			GitImplementation: "go-git",
		},
	}
	By("creating the GitRepository resource in the cluster.")
	It("should be created successfully.")
	g.Expect(k8sClient.Create(ctx, &testRepo)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &testRepo)).Should(Succeed()) }()

	Given("the GitRepository's reconciled status.")
	By("setting the GitRepository's status, with the downloadable BLOB's URL, and the correct checksum.")
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
		URL: server.URL() + "/file.tar.gz",
		Artifact: &sourcev1.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/file.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Checksum:       "80ddfd18eb96f7d31cadc1a8a5171c6e2d95df3f6c23b0ed9cd8dddf6dba1406",
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}

	It("should be updated successfully.")
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	It("should be retrievable by the k8s client.")
	By("getting it with the k0s client and succeed.")
	gitRepoKey := types.NamespacedName{Namespace: "flux-system", Name: sourceName}
	createdRepo := sourcev1.GitRepository{}
	g.Expect(k8sClient.Get(ctx, gitRepoKey, &createdRepo)).Should(Succeed())

	Given("a Terraform resource with auto approve, backend configured, attached to the given GitRepository.")
	By("creating a new TF resource and attaching to the repo via `sourceRef`.")
	backendConfig := fmt.Sprintf(`
	backend "s3" {
		bucket                      = "s3-terraform-state1"
		key                         = "dev/terraform.tfstate"
		region                      = "us-east-1"
		endpoint                    = "http://localhost:4566"
		skip_credentials_validation = true
		skip_metadata_api_check     = true
		force_path_style            = true
		dynamodb_table              = "terraformlock"
		dynamodb_endpoint           = "http://localhost:4566"
		encrypt                     = true
	  }
	`)
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			ApprovePlan: "auto",
			BackendConfig: &infrav1.BackendConfigSpec{
				CustomConfiguration: backendConfig,
			},
			Path: "./terraform-hello-world-example",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
		},
	}
	It("should be created and attached successfully.")
	g.Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &helloWorldTF)).Should(Succeed()) }()

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

}
