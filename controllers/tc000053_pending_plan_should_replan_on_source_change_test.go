package controllers

import (
	"os"
	"testing"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000053_pending_plan_should_replan_on_source_change_test(t *testing.T) {
	Spec("This spec describes the behaviour of a Terraform resource with a pending plan when the source revision changes.")
	It("should be reconciled to become planned.")
	It("should wait for a manual approval.")
	It("when the source revision changes, it should automatically re-plan without requiring a replan annotation.")
	It("then should be reconciled to the applied state after approving the new plan.")

	const (
		sourceName    = "gr-pending-plan-replan-on-source-change"
		terraformName = "tf-pending-plan-replan-on-source-change"
	)
	ctx := t.Context()
	g := NewWithT(t)

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
				Branch: "master",
			},
			Interval: metav1.Duration{Duration: time.Second * 30},
		},
	}

	By("creating the GitRepository resource in the cluster.")
	It("should be created successfully.")
	g.Expect(k8sClient.Create(ctx, &testRepo)).Should(Succeed())
	defer waitResourceToBeDelete(g, &testRepo)

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
				Message:            "Fetched revision: master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			},
		},
		Artifact: &meta.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/file.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Digest:         "sha256:80ddfd18eb96f7d31cadc1a8a5171c6e2d95df3f6c23b0ed9cd8dddf6dba1406",
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}

	It("should be updated successfully.")
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	By("checking that the status and its URL gets reconciled.")
	gitRepoKey := types.NamespacedName{Namespace: "flux-system", Name: sourceName}
	createdRepo := sourcev1.GitRepository{}
	g.Expect(k8sClient.Get(ctx, gitRepoKey, &createdRepo)).Should(Succeed())

	Given("a Terraform resource with manual approval, attached to the given GitRepository resource")
	By("creating a new TF resource without specifying the .spec.approvePlan field.")
	By("attaching the TF resource to the repo via `sourceRef`.")
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			// Note that we do not specify the `ApprovePlan` field - this is manual approval mode.
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
	tfplanSecret := corev1.Secret{}
	defer waitResourceToBeDelete(g, &tfplanSecret) // must be deleted after TF resource
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

	By("checking that the TF's status conditions got reconciled.")
	g.Eventually(func() int {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return -1
		}
		return len(createdHelloWorldTF.Status.Conditions)
	}, timeout*3, interval).ShouldNot(BeZero())

	Given("the plan id is the `plan` plus the branch name (master) plus the first 10 chars of the commit hash.")
	const firstPlanId = "plan-master-b8e362c206"

	By("checking that the planned status of the TF is created successfully.")
	By("checking the reason is `TerraformPlannedWithChanges`.")
	By("checking the pending plan is the $firstPlanId.")
	g.Eventually(func() map[string]any {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == "Plan" {
				return map[string]any{
					"Type":    c.Type,
					"Reason":  c.Reason,
					"Pending": createdHelloWorldTF.Status.Plan.Pending,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]any{
		"Type":    infrav1.ConditionTypePlan,
		"Reason":  "TerraformPlannedWithChanges",
		"Pending": firstPlanId,
	}))

	By("checking the message of the ready status contains $firstPlanId.")
	g.Eventually(func() map[string]any {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == "Ready" {
				return map[string]any{
					"Type":    c.Type,
					"Reason":  c.Reason,
					"Message": c.Message,
				}
			}
		}
		return nil
	}, timeout*3, interval).Should(Equal(map[string]any{
		"Type":    "Ready",
		"Reason":  "TerraformPlannedWithChanges",
		"Message": "Plan generated: set approvePlan: \"plan-master-b8e362c206\" to approve this plan.",
	}))

	By("checking that the planned secret is created.")
	By("checking that the annotation of the planned secret is the $firstPlanId.")
	tfplanKey := types.NamespacedName{Namespace: "flux-system", Name: "tfplan-default-" + terraformName}
	g.Eventually(func() map[string]any {
		err := k8sClient.Get(ctx, tfplanKey, &tfplanSecret)
		if err != nil {
			return nil
		}
		return map[string]any{
			"SavedPlan":             tfplanSecret.Annotations["savedPlan"],
			"TFPlanEmpty":           string(tfplanSecret.Data["tfplan"]) == "",
			"HasEncodingAnnotation": tfplanSecret.Annotations["encoding"] == "gzip",
		}
	}, timeout, interval).Should(Equal(map[string]any{
		"SavedPlan":             firstPlanId,
		"TFPlanEmpty":           false,
		"HasEncodingAnnotation": true,
	}))

	// This is the key part of the test: change the source revision while the plan
	// is still pending approval and the user has NOT set approvePlan to anything.
	// The controller should detect that the source has changed and automatically
	// invalidate the stale pending plan, then re-plan with the new revision.
	By("changing source to a new revision while the plan is still pending approval")
	updatedTime = time.Now()
	testRepo.Status = sourcev1.GitRepositoryStatus{
		ObservedGeneration: int64(2),
		Conditions: []metav1.Condition{
			{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Time{Time: updatedTime},
				Reason:             "GitOperationSucceed",
				Message:            "Fetched revision: master/ed22ced771a0056455a2fbb8e362c206e3d0cbb7",
			},
		},
		Artifact: &meta.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/ed22ced771a0056455a2fbb8e362c206e3d0cbb7.tar.gz",
			URL:            server.URL() + "/2222.tar.gz",
			Revision:       "master/ed22ced771a0056455a2fbb8e362c206e3d0cbb7",
			Digest:         "sha256:525802635a47a5ae3f9c058a2b958aac0daef08efbe100a4fc16833df5201b94",
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	// NOTE: We intentionally do NOT set approvePlan to "replan-..." here.
	// The controller should automatically detect the revision change and re-plan.

	const secondPlanId = "plan-master-ed22ced771"

	By("checking that the controller automatically re-plans with the new revision.")
	By("checking the pending plan is now the $secondPlanId derived from the new commit.")
	g.Eventually(func() map[string]any {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == "Plan" {
				return map[string]any{
					"Type":    c.Type,
					"Reason":  c.Reason,
					"Pending": createdHelloWorldTF.Status.Plan.Pending,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]any{
		"Type":    infrav1.ConditionTypePlan,
		"Reason":  "TerraformPlannedWithChanges",
		"Pending": secondPlanId,
	}))

	By("checking the message of the ready status now contains the new $secondPlanId.")
	g.Eventually(func() map[string]any {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == "Ready" {
				return map[string]any{
					"Type":    c.Type,
					"Reason":  c.Reason,
					"Message": c.Message,
				}
			}
		}
		return nil
	}, timeout*3, interval).Should(Equal(map[string]any{
		"Type":    "Ready",
		"Reason":  "TerraformPlannedWithChanges",
		"Message": "Plan generated: set approvePlan: \"plan-master-ed22ced771\" to approve this plan.",
	}))

	By("approving the new plan with the $secondPlanId.")
	createdHelloWorldTF.Spec.ApprovePlan = secondPlanId
	g.Expect(k8sClient.Update(ctx, &createdHelloWorldTF)).Should(Succeed())

	It("should continue the reconciliation process to the apply state.")
	By("checking that the applied status reason is TerraformAppliedSucceed.")
	By("checking that the last applied plan is the $secondPlanId.")
	g.Eventually(func() map[string]any {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == infrav1.ConditionTypeApply {
				return map[string]any{
					"Type":            c.Type,
					"Reason":          c.Reason,
					"LastAppliedPlan": createdHelloWorldTF.Status.Plan.LastApplied,
				}
			}
		}
		return nil
	}, timeout, interval).Should(Equal(map[string]any{
		"Type":            infrav1.ConditionTypeApply,
		"Reason":          infrav1.TFExecApplySucceedReason,
		"LastAppliedPlan": secondPlanId,
	}))

	It("should contain a list of available outputs in the status.")
	By("checking that .status.availableOutput in the TF resource.")
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
		}, timeout, interval).Should(Equal("secrets \"tfstate-default-tf-pending-plan-replan-on-source-change\" not found"))
	} else {
		// TODO there's must be the default tfstate secret
	}
}
