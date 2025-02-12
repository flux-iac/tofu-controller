package controllers

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	. "github.com/onsi/gomega"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000012_src_ocirepository_no_outputs_test(t *testing.T) {
	Spec("This spec describes the behaviour of a Terraform resource which is stored in an OCIRepository. There is no backend and auto-approve is enabled.")
	It("should be reconciled to have available outputs.")

	var (
		sourceName    = "test-tf-controller-src-ocirepository-no-output"
		terraformName = "src-ocirepository-helloworld-no-outputs-" + rand.String(6)
	)
	g := NewWithT(t)
	ctx := context.Background()

	Given("an OCIRepository")
	By("defining a new OCIRepository resource.")
	testOCIRepository := sourcev1b2.OCIRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sourceName,
			Namespace: "flux-system",
		},
		Spec: sourcev1b2.OCIRepositorySpec{
			URL:      "oci://ghcr.io/flux-iac/aws-primitive-modules",
			Provider: "generic",
			Reference: &sourcev1b2.OCIRepositoryRef{
				Tag: "v4.38.0-v1alpha11",
			},
			Interval: metav1.Duration{Duration: time.Second * 30},
		},
	}

	By("creating the OCIRepository resource in the cluster.")
	It("should be created successfully.")
	g.Expect(k8sClient.Create(ctx, &testOCIRepository)).Should(Succeed())
	defer waitResourceToBeDelete(g, &testOCIRepository)

	Given("the OCIRepository's reconciled status.")
	By("setting the OCIRepository's status, with the downloadable BLOB's URL, and the correct checksum.")
	updatedTime := time.Now()
	testOCIRepository.Status = sourcev1b2.OCIRepositoryStatus{
		ObservedGeneration: int64(1),
		Conditions: []metav1.Condition{
			{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Time{Time: updatedTime},
				Reason:             "Succeeded",
				Message:            "stored artifact for digest 'v4.38.0-v1alpha11@sha256:6033f3b9fb6458dda7f432ea5ce1a8dbb9560c2388d612f0d1036ba03cf2e063'",
			},
			{
				Type:               "ArtifactInStorage",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Time{Time: updatedTime},
				Reason:             "Succeeded",
				Message:            "stored artifact for digest 'v4.38.0-v1alpha11@sha256:6033f3b9fb6458dda7f432ea5ce1a8dbb9560c2388d612f0d1036ba03cf2e063'",
			},
		},
		Artifact: &sourcev1.Artifact{
			Path:           fmt.Sprintf("ocirepository/flux-system/%s/sha256:6033f3b9fb6458dda7f432ea5ce1a8dbb9560c2388d612f0d1036ba03cf2e063.tar.gz", sourceName),
			URL:            server.URL() + "/file.tar.gz",
			Revision:       "v4.38.0-v1alpha11@sha256:6033f3b9fb6458dda7f432ea5ce1a8dbb9560c2388d612f0d1036ba03cf2e063",
			Digest:         "sha256:f1b07a5d730814b59ce6be5565f4dd27b183826148c79331fb591b630a7c732a",
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}

	It("should be updated successfully.")
	g.Expect(k8sClient.Status().Update(ctx, &testOCIRepository)).Should(Succeed())

	Given("a Terraform resource with auto approve, attached to the given OCIRepository resource.")
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
				Kind:      "OCIRepository",
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
		return err == nil
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
		"Type":    infrav1.ConditionTypePlan,
		"Reason":  "TerraformPlannedWithChanges",
		"Message": "Plan generated",
	}))

	It("should generate the Secret containing the plan named with the artifact revision")
	By("checking that the Secret contains plan-v4.38.0-v1alpha11-6033f3b9fb in its labels.")
	tfplanKey := types.NamespacedName{Namespace: "flux-system", Name: "tfplan-default-" + terraformName}
	tfplanSecret := corev1.Secret{}
	g.Eventually(func() map[string]interface{} {
		err := k8sClient.Get(ctx, tfplanKey, &tfplanSecret)
		if err != nil {
			return nil
		}
		return map[string]interface{}{
			"SavedPlan":             tfplanSecret.Annotations["savedPlan"],
			"Is TFPlan empty ?":     string(tfplanSecret.Data["tfplan"]) == "",
			"HasEncodingAnnotation": tfplanSecret.Annotations["encoding"] == "gzip",
		}
	}, timeout, interval).Should(Equal(map[string]interface{}{
		"SavedPlan":             "plan-v4.38.0-v1alpha11-6033f3b9fb",
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
		"Type":            infrav1.ConditionTypeApply,
		"Reason":          infrav1.TFExecApplySucceedReason,
		"Message":         "Applied successfully",
		"LastAppliedPlan": "plan-v4.38.0-v1alpha11-6033f3b9fb",
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
		}, timeout, interval).Should(Equal(fmt.Sprintf("secrets \"tfstate-default-%s\" not found", terraformName)))
	} else {
		// TODO there's must be the default tfstate secret
		It("should produce a Secret because the controller runs in Kubernetes.")
		By("checking that the Secret is available.")
		tfStateKey := types.NamespacedName{Namespace: "flux-system", Name: "tfstate-default-" + terraformName}
		tfStateSecret := corev1.Secret{}
		g.Eventually(func() string {
			err := k8sClient.Get(ctx, tfStateKey, &tfStateSecret)
			if err != nil {
				return err.Error()
			}
			return tfStateSecret.Name
		}, timeout, interval).Should(Equal("tfstate-default-" + terraformName))
	}

}
