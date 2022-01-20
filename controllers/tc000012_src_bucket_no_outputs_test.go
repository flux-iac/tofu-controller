package controllers

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	infrav1 "github.com/chanwit/tf-controller/api/v1alpha1"
	. "github.com/onsi/gomega"

	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000012_src_bucket_no_outputs_test(t *testing.T) {
	Spec("This spec describes the behaviour of a Terraform resource which is stored in an S3-compatible bucket. There is no backend and auto-approve is enabled.")
	It("should be reconciled to have available outputs.")

	const (
		sourceName    = "test-tf-controller-src-bucket-no-output"
		terraformName = "src-bucket-helloworld-no-outputs"
	)
	g := NewWithT(t)
	ctx := context.Background()

	Given("a Bucket")
	By("defining a new Bucket resource.")
	testBucket := sourcev1.Bucket{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sourceName,
			Namespace: "flux-system",
		},
		Spec: sourcev1.BucketSpec{
			BucketName: "test-flux-tf-bucket",
			Provider:   "generic",
			Interval:   metav1.Duration{Duration: time.Second * 30},
		},
	}

	By("creating the Bucket resource in the cluster.")
	It("should be created successfully.")
	g.Expect(k8sClient.Create(ctx, &testBucket)).Should(Succeed())
	defer func() { g.Expect(k8sClient.Delete(ctx, &testBucket)).Should(Succeed()) }()

	Given("the Bucket's reconciled status.")
	By("setting the Bucket's status, with the downloadable BLOB's URL, and the correct checksum.")
	updatedTime := time.Now()
	testBucket.Status = sourcev1.BucketStatus{
		ObservedGeneration: int64(1),
		Conditions: []metav1.Condition{
			{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Time{Time: updatedTime},
				Reason:             "BucketOperationSucceed",
				Message:            "Fetched revision: 822c3dd335579b435b5ada924d6f38b227412a5c",
			},
		},
		URL: server.URL() + "/file.tar.gz",
		Artifact: &sourcev1.Artifact{
			Path:           fmt.Sprintf("bucket/flux-system/%s/822c3dd335579b435b5ada924d6f38b227412a5c.tar.gz", sourceName),
			URL:            server.URL() + "/file.tar.gz",
			Revision:       "822c3dd335579b435b5ada924d6f38b227412a5c",
			Checksum:       "80ddfd18eb96f7d31cadc1a8a5171c6e2d95df3f6c23b0ed9cd8dddf6dba1406",
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}

	It("should be updated successfully.")
	g.Expect(k8sClient.Status().Update(ctx, &testBucket)).Should(Succeed())

	Given("a Terraform resource with auto approve, attached to the given Bucket resource.")
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
				Kind:      "Bucket",
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

	It("should be planned.")
	By("checking that the Plan's reason of the TF resource become `TerraformPlannedWithChanges`.")
	g.Eventually(func() interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == "Plan" {
				return map[string]interface{}{
					"Type":    c.Type,
					"Reason":  c.Reason,
					"Message": c.Message,
				}
			}
		}
		return createdHelloWorldTF.Status
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":    "Plan",
		"Reason":  "TerraformPlannedWithChanges",
		"Message": "Plan generated",
	}))

	It("should generate the Secret containing the plan named with the artifact revision")
	By("checking that the Secret contains plan-822c3dd335579b435b5ada924d6f38b227412a5c in its labels.")
	tfplanKey := types.NamespacedName{Namespace: "flux-system", Name: "tfplan-default-" + terraformName}
	tfplanSecret := corev1.Secret{}
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, tfplanKey, &tfplanSecret)
		if err != nil {
			return nil
		}
		return map[string]interface{}{
			"SavedPlan":             tfplanSecret.Labels["savedPlan"],
			"Is TFPlan empty ?":     string(tfplanSecret.Data["tfplan"]) == "",
			"HasEncodingAnnotation": tfplanSecret.Annotations["encoding"] != "" && tfplanSecret.Annotations["encoding"] == "gzip",
		}
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"SavedPlan":             "plan-822c3dd335579b435b5ada924d6f38b227412a5c",
		"Is TFPlan empty ?":     false,
		"HasEncodingAnnotation": true,
	}))

	It("should contain an Apply condition saying that the plan were apply successfully.")
	By("checking that the reason of the Apply condition is TerraformAppliedSucceed, and the LastAppliedPlan is the plan.")
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == "Apply" {
				return map[string]interface{}{
					"Type":            c.Type,
					"Reason":          c.Reason,
					"Message":         c.Message,
					"LastAppliedPlan": createdHelloWorldTF.Status.Plan.LastApplied,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"Type":            "Apply",
		"Reason":          "TerraformAppliedSucceed",
		"Message":         "Applied successfully",
		"LastAppliedPlan": "plan-822c3dd335579b435b5ada924d6f38b227412a5c",
	}))

	It("should have an available output.")
	By("checking that the Terraform resource's .status.availableOutputs contains hello_world as an output name.")
	g.Eventually(func() []string {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		return createdHelloWorldTF.Status.AvailableOutputs
	}, timeout, interval).Should(Equal([]string{"hello_world"}))

	if os.Getenv("DISABLE_TF_K8S_BACKEND") == "1" {
		It("should not produce a Secret because the controller runs locally, outside Kubernetes.")
		By("checking there are no secret generated by default.")
		tfStateKey := types.NamespacedName{Namespace: "flux-system", Name: "tfstate-default-" + terraformName}
		tfStateSecret := corev1.Secret{}
		g.Eventually(func() string {
			err := k8sClient.Get(ctx, tfStateKey, &tfStateSecret)
			if err != nil {
				return err.Error()
			}
			return tfStateSecret.Name
		}, timeout, interval).Should(Equal("secrets \"tfstate-default-src-bucket-helloworld-no-outputs\" not found"))
	} else {
		// TODO there's must be the default tfstate secret
	}

}
