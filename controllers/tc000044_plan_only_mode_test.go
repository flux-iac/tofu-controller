package controllers

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000044_plan_only_mode_test(t *testing.T) {
	Spec("This spec describes the behaviour of a Terraform resource when PlanOnly is set")
	It("should be reconciled and write human readable plan output.")

	const (
		sourceName    = "test-tf-controller-plan-only"
		terraformName = "helloworld-plan-only"
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

	By("checking that the status and its URL gets reconciled.")
	gitRepoKey := types.NamespacedName{Namespace: "flux-system", Name: sourceName}
	createdRepo := &sourcev1.GitRepository{}
	g.Expect(k8sClient.Get(ctx, gitRepoKey, createdRepo)).Should(Succeed())

	Given("a Terraform resource with auto approve, without the list of outputs, attached to the given GitRepository resource")
	By("creating a new TF resource and attaching to the repo via `sourceRef`.")
	By("not specifying the outputs list of .spec.writeOutputsToSecret.")
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			PlanOnly: true,
			Path:     "./terraform-hello-world-example",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
			Interval:          metav1.Duration{Duration: time.Second * 10},
			StoreReadablePlan: "human",
			WriteOutputsToSecret: &infrav1.WriteOutputsToSecretSpec{
				Name: "tf-output-" + terraformName,
				// NOTE comment out only. Please not remove this line: Outputs: []string{},
			},
		},
	}
	It("should be created and attached successfully.")
	g.Expect(k8sClient.Create(ctx, &helloWorldTF)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &helloWorldTF)).Should(Succeed()) }()

	By("checking that the TF resource existed inside the cluster.")
	helloWorldTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName}
	createdHelloWorldTF := infrav1.Terraform{}
	g.Expect(k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)).To(Succeed())

	It("should be reconciled and produce the correct output secret.")
	By("checking that the named output secret contains all outputs.")
	outputKey := types.NamespacedName{Namespace: "flux-system", Name: "tfplan-default-" + terraformName}
	planOutput := corev1.ConfigMap{}
	g.Eventually(func() (int, error) {
		err := k8sClient.Get(ctx, outputKey, &planOutput)
		if err != nil {
			return -1, err
		}
		return len(planOutput.Data), nil
	}, timeout, interval).Should(Equal(1))

	By("checking that the output ConfigMap contains the correct output data, provisioned by the TF resource.")

	g.Expect(planOutput.Name).To(Equal("tfplan-default-" + terraformName))
	g.Expect(planOutput.Namespace).To(Equal("flux-system"))
	g.Expect(string(planOutput.OwnerReferences[0].UID)).To(Equal(string(createdHelloWorldTF.UID)))
	g.Expect(string(planOutput.Data["tfplan"])).To(ContainSubstring(`+ hello_world = "Hello, World!"`))

	It("should be stopped.")
	By("checking the ready condition is still Plan within 5 seconds.")

	var readyCondition *metav1.Condition
	g.Eventually(func() *metav1.Condition {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}

		readyCondition = meta.FindStatusCondition(createdHelloWorldTF.Status.Conditions, "Ready")

		return readyCondition
	}, timeout, interval).ShouldNot(BeNil())

	g.Expect(*readyCondition).To(
		gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"Type":    Equal("Ready"),
			"Reason":  Equal(infrav1.PlannedWithChangesReason),
			"Message": Equal("Plan generated: This object is in the plan only mode."),
			"Status":  Equal(metav1.ConditionUnknown),
		}),
	)
}
