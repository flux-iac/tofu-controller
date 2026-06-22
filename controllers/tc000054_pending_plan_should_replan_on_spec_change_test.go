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

func Test_000054_pending_plan_should_replan_on_spec_change_test(t *testing.T) {
	Spec("This spec describes the behaviour of a Terraform resource with a pending plan when the Terraform spec changes while the source revision stays the same.")
	It("should be reconciled to become planned.")
	It("should wait for a manual approval.")
	It("when the Terraform spec changes (generation bump) while the source revision is unchanged, it should automatically re-plan without requiring a replan annotation.")
	It("then should be reconciled to the applied state after approving the re-planned plan.")

	const (
		sourceName    = "gr-pending-plan-replan-on-spec-change"
		terraformName = "tf-pending-plan-replan-on-spec-change"
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
	defer func() {
		if tfplanSecret.Name != "" {
			waitResourceToBeDelete(g, &tfplanSecret)
		}
	}() // must be deleted after TF resource
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
	// Plan id is derived from the source revision, so it does NOT change when only
	// the Terraform spec is modified. The fix re-plans against the same plan id but
	// with refreshed content from the new spec.
	const planId = "plan-master-b8e362c206"

	By("checking that the planned status of the TF is created successfully.")
	By("checking the reason is `TerraformPlannedWithChanges`.")
	By("checking the pending plan is the $planId.")
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
		"Pending": planId,
	}))

	By("checking that LastPlannedGeneration is recorded against the initial spec generation.")
	g.Expect(k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)).Should(Succeed())
	initialGeneration := createdHelloWorldTF.Generation
	g.Expect(createdHelloWorldTF.Status.LastPlannedGeneration).To(Equal(initialGeneration))

	// This is the key part of the test: modify the Terraform spec (bumping
	// metadata.generation) while leaving the GitRepository revision unchanged.
	// The controller should detect that the spec has changed and automatically
	// invalidate the stale pending plan, then re-plan against the updated spec.
	By("changing the Terraform spec while the source revision is unchanged")
	g.Eventually(func() error {
		if err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF); err != nil {
			return err
		}
		// Bumping Interval is sufficient to advance metadata.generation.
		createdHelloWorldTF.Spec.Interval = metav1.Duration{Duration: time.Second * 15}
		return k8sClient.Update(ctx, &createdHelloWorldTF)
	}, timeout, interval).Should(Succeed())

	// NOTE: We intentionally do NOT set approvePlan to "replan-..." here and we do
	// NOT change the GitRepository revision. The controller should automatically
	// detect the spec change (generation bump) and re-plan.

	By("waiting for the controller to observe the new generation and re-plan.")
	g.Eventually(func() bool {
		if err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF); err != nil {
			return false
		}
		// Re-plan has completed when LastPlannedGeneration catches up to the
		// new spec generation.
		return createdHelloWorldTF.Status.LastPlannedGeneration > initialGeneration &&
			createdHelloWorldTF.Status.LastPlannedGeneration == createdHelloWorldTF.Generation
	}, timeout*3, interval).Should(BeTrue())

	By("checking that the re-planned status is still TerraformPlannedWithChanges with the same plan id (source unchanged).")
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
		"Pending": planId,
	}))

	By("approving the re-planned plan.")
	g.Eventually(func() error {
		if err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF); err != nil {
			return err
		}
		createdHelloWorldTF.Spec.ApprovePlan = planId
		return k8sClient.Update(ctx, &createdHelloWorldTF)
	}, timeout, interval).Should(Succeed())

	It("should continue the reconciliation process to the apply state.")
	By("checking that the applied status reason is TerraformAppliedSucceed.")
	By("checking that the last applied plan is the $planId.")
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
		"LastAppliedPlan": planId,
	}))

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
		}, timeout, interval).Should(Equal("secrets \"tfstate-default-" + terraformName + "\" not found"))
	}
}
