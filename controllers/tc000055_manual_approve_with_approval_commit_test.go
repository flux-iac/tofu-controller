package controllers

import (
	"testing"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// +kubebuilder:docs-gen:collapse=Imports

// The CR and the configuration share a Git repository: the approval commit moves the
// revision, and plan IDs derive from it, so an approval reaching the controller along
// with a new revision must still apply the plan it approved.
// https://github.com/flux-iac/tofu-controller/issues/1878
func Test_000055_manual_approve_with_approval_commit_test(t *testing.T) {
	Spec("This spec describes the behaviour of a Terraform resource approved by a commit that also changes the source revision.")
	It("should be reconciled to become planned.")
	It("should apply the approved plan, not a plan regenerated from the approval commit.")
	It("then should reconcile the new source revision on its own.")

	const (
		sourceName    = "gr-manual-approve-with-approval-commit"
		terraformName = "tf-manual-approve-with-approval-commit"

		plannedRevision = "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb"
		// served from the same tarball and digest as plannedRevision: the approval
		// commit must look like a change to the CR alone, not to the configuration
		approvalRevision = "master/aaaa111122223333444455556666777788889999"

		plannedPlanId  = "plan-master-b8e362c206"
		approvalPlanId = "plan-master-aaaa111122"

		artifactDigest = "sha256:80ddfd18eb96f7d31cadc1a8a5171c6e2d95df3f6c23b0ed9cd8dddf6dba1406"
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
				Message:            "Fetched revision: " + plannedRevision,
			},
		},
		Artifact: &meta.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/file.tar.gz",
			Revision:       plannedRevision,
			Digest:         artifactDigest,
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}

	It("should be updated successfully.")
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	Given("a Terraform resource with manual approval, attached to the given GitRepository resource")
	By("creating a new TF resource without specifying the .spec.approvePlan field.")
	By("attaching the TF resource to the repo via `sourceRef`.")
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			// Note that we do not specify the `ApprovePlan` field
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

	helloWorldTFKey := types.NamespacedName{Namespace: "flux-system", Name: terraformName}
	createdHelloWorldTF := infrav1.Terraform{}

	By("checking that the pending plan is generated from the first revision.")
	g.Eventually(func() map[string]any {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == infrav1.ConditionTypePlan {
				return map[string]any{
					"Reason":              c.Reason,
					"Pending":             createdHelloWorldTF.Status.Plan.Pending,
					"LastPlannedRevision": createdHelloWorldTF.Status.LastPlannedRevision,
				}
			}
		}
		return nil
	}, timeout*3, interval).Should(Equal(map[string]any{
		"Reason":              "TerraformPlannedWithChanges",
		"Pending":             plannedPlanId,
		"LastPlannedRevision": plannedRevision,
	}))

	By("checking that the planned secret is saved under the pending plan id.")
	tfplanKey := types.NamespacedName{Namespace: "flux-system", Name: "tfplan-default-" + terraformName}
	g.Eventually(func() string {
		err := k8sClient.Get(ctx, tfplanKey, &tfplanSecret)
		if err != nil {
			return err.Error()
		}
		return tfplanSecret.Annotations["savedPlan"]
	}, timeout, interval).Should(Equal(plannedPlanId))

	Given("an approval commit, which changes the source revision and the approvePlan field at once")
	By("suspending the TF resource, so that both changes are observed by the same reconciliation.")
	patch := client.MergeFrom(createdHelloWorldTF.DeepCopy())
	createdHelloWorldTF.Spec.Suspend = true
	g.Expect(k8sClient.Patch(ctx, &createdHelloWorldTF, patch)).Should(Succeed())

	By("moving the source to the revision of the approval commit, with an unchanged Terraform configuration.")
	g.Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "flux-system", Name: sourceName}, &testRepo)).Should(Succeed())
	testRepo.Status = sourcev1.GitRepositoryStatus{
		ObservedGeneration: int64(2),
		Conditions: []metav1.Condition{
			{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Time{Time: updatedTime},
				Reason:             "GitOperationSucceed",
				Message:            "Fetched revision: " + approvalRevision,
			},
		},
		Artifact: &meta.Artifact{
			Path:           "gitrepository/flux-system/test-tf-controller/aaaa111122223333444455556666777788889999.tar.gz",
			URL:            server.URL() + "/file.tar.gz",
			Revision:       approvalRevision,
			Digest:         artifactDigest,
			LastUpdateTime: metav1.Time{Time: updatedTime},
		},
	}
	g.Expect(k8sClient.Status().Update(ctx, &testRepo)).Should(Succeed())

	By("approving the pending plan and resuming, as the approval commit would do.")
	patch = client.MergeFrom(createdHelloWorldTF.DeepCopy())
	createdHelloWorldTF.Spec.ApprovePlan = plannedPlanId
	createdHelloWorldTF.Spec.Suspend = false
	g.Expect(k8sClient.Patch(ctx, &createdHelloWorldTF, patch)).Should(Succeed())

	It("should apply the approved plan, although the source revision has moved on.")
	By("checking that the last applied plan is the approved one, recorded against the revision it was planned from.")
	g.Eventually(func() map[string]any {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == infrav1.ConditionTypeApply {
				return map[string]any{
					"Reason":              c.Reason,
					"LastAppliedPlan":     createdHelloWorldTF.Status.Plan.LastApplied,
					"LastAppliedRevision": createdHelloWorldTF.Status.LastAppliedRevision,
				}
			}
		}
		return nil
	}, timeout*3, interval).Should(Equal(map[string]any{
		"Reason":              infrav1.TFExecApplySucceedReason,
		"LastAppliedPlan":     plannedPlanId,
		"LastAppliedRevision": plannedRevision,
	}))

	It("should then reconcile the revision of the approval commit on its own.")
	// the test backend keeps no state, so this revision always plans with changes;
	// what matters is that it gets a plan id of its own to approve.
	By("checking that the approval revision is planned separately.")
	g.Eventually(func() map[string]any {
		err := k8sClient.Get(ctx, helloWorldTFKey, &createdHelloWorldTF)
		if err != nil {
			return nil
		}
		for _, c := range createdHelloWorldTF.Status.Conditions {
			if c.Type == infrav1.ConditionTypePlan {
				return map[string]any{
					"Reason":              c.Reason,
					"Pending":             createdHelloWorldTF.Status.Plan.Pending,
					"LastPlannedRevision": createdHelloWorldTF.Status.LastPlannedRevision,
				}
			}
		}
		return nil
	}, timeout*3, interval).Should(Equal(map[string]any{
		"Reason":              "TerraformPlannedWithChanges",
		"Pending":             approvalPlanId,
		"LastPlannedRevision": approvalRevision,
	}))
}
