package controllers

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

// +kubebuilder:docs-gen:collapse=Imports

// Test_000362_plan_lock_disabled_test verifies that a Terraform resource with
// spec.plan.lock set to false is accepted and reconciles to a successful plan.
// The plan-only knobs flow to the runner via the serialised CR spec, so this is
// the end-to-end confirmation that the field is wired through; the concrete
// `-lock=false` flag value is verified manually via the runner logs.
func Test_000362_plan_lock_disabled_test(t *testing.T) {
	Spec("This spec describes planning behaviour when the plan state lock is disabled via spec.plan.lock.")
	It("should be reconciled and planned successfully with the lock disabled for the plan phase.")

	const (
		sourceName    = "test-tf-controller-plan-lock-disabled"
		terraformName = "helloworld-plan-lock-disabled"
	)
	g := NewWithT(t)
	ctx := t.Context()

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

	By("checking that the status and its URL gets reconciled")
	gitRepoKey := types.NamespacedName{Namespace: "flux-system", Name: sourceName}
	createdRepo := sourcev1.GitRepository{}
	g.Expect(k8sClient.Get(ctx, gitRepoKey, &createdRepo)).Should(Succeed())

	Given("a Terraform resource with spec.plan.lock disabled, attached to the given GitRepository")
	By("creating a new TF resource with `spec.plan.lock: false` and attaching it to the repo via `sourceRef`.")
	helloWorldTF := infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:      terraformName,
			Namespace: "flux-system",
		},
		Spec: infrav1.TerraformSpec{
			Path:     "./terraform-hello-world-example",
			Interval: metav1.Duration{Duration: 3 * time.Second},
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      sourceName,
				Namespace: "flux-system",
			},
			Plan: &infrav1.PlanSpec{
				Lock: ptr.To(false),
			},
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

	It("should persist the plan lock setting on the spec.")
	By("checking that spec.plan.lock is stored as false.")
	g.Expect(createdHelloWorldTF.Spec.Plan).ShouldNot(BeNil())
	g.Expect(createdHelloWorldTF.Spec.Plan.Lock).ShouldNot(BeNil())
	g.Expect(*createdHelloWorldTF.Spec.Plan.Lock).Should(BeFalse())

	It("should be planned successfully with the lock disabled.")
	By("checking that the Plan's reason of the TF resource becomes `TerraformPlannedWithChanges`.")
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
		"Pending": "plan-master-b8e362c206",
	}))
}
