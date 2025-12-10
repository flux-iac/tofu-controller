package controllers

import (
	"testing"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestShouldReconcileSkipsWhenIntervalNotElapsed(t *testing.T) {
	Spec("This spec covers skipping reconciliation when the last plan is still within the interval window.")
	It("should return false with a positive requeue duration.")

	g := NewWithT(t)
	reconciler := &TerraformReconciler{}

	lastPlan := time.Now().Add(-6 * time.Hour)
	tf := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 1,
		},
		Spec: infrav1.TerraformSpec{
			Interval: metav1.Duration{Duration: 24 * time.Hour},
		},
		Status: infrav1.TerraformStatus{
			LastPlanAt:            &metav1.Time{Time: lastPlan},
			LastAttemptedRevision: "main/1234",
			LastPlannedRevision:   "main/1234",
			ObservedGeneration:    1,
		},
	}

	shouldReconcile, reason, requeueAfter := reconciler.shouldReconcile(tf, nil)
	g.Expect(shouldReconcile).To(BeFalse())
	g.Expect(reason).To(Equal("interval has not elapsed since last plan"))
	g.Expect(requeueAfter).To(BeNumerically(">", 17*time.Hour))
	g.Expect(requeueAfter).To(BeNumerically("<=", 24*time.Hour))
}

func TestShouldReconcileWhenIntervalElapsed(t *testing.T) {
	Spec("This spec covers reconciling once the interval has fully elapsed.")
	It("should return true with zero requeue duration.")

	g := NewWithT(t)
	reconciler := &TerraformReconciler{}

	lastPlan := time.Now().Add(-25 * time.Hour)
	tf := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 1,
		},
		Spec: infrav1.TerraformSpec{
			Interval: metav1.Duration{Duration: 24 * time.Hour},
		},
		Status: infrav1.TerraformStatus{
			LastPlanAt:         &metav1.Time{Time: lastPlan},
			ObservedGeneration: 1,
		},
	}

	shouldReconcile, reason, requeueAfter := reconciler.shouldReconcile(tf, nil)
	g.Expect(shouldReconcile).To(BeTrue())
	g.Expect(reason).To(Equal(""))
	g.Expect(requeueAfter).To(Equal(time.Duration(0)))
}

func TestShouldReconcileWhenGenerationChanged(t *testing.T) {
	Spec("This spec covers reconciling when the object generation has changed.")
	It("should return true even if the interval has not elapsed.")

	g := NewWithT(t)
	reconciler := &TerraformReconciler{}

	tf := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 2,
		},
		Spec: infrav1.TerraformSpec{
			Interval: metav1.Duration{Duration: 24 * time.Hour},
		},
		Status: infrav1.TerraformStatus{
			LastPlanAt:         &metav1.Time{Time: time.Now()},
			ObservedGeneration: 1,
		},
	}

	shouldReconcile, reason, requeueAfter := reconciler.shouldReconcile(tf, nil)
	g.Expect(shouldReconcile).To(BeTrue())
	g.Expect(reason).To(Equal("terraform generation has changed"))
	g.Expect(requeueAfter).To(Equal(time.Duration(0)))
}

func TestShouldReconcileWhenRequestedViaAnnotation(t *testing.T) {
	Spec("This spec covers reconciling immediately when the reconcile request annotation is set.")
	It("should return true even if the interval has not elapsed and the generation is the same.")

	g := NewWithT(t)
	reconciler := &TerraformReconciler{}

	tf := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 1,
			Annotations: map[string]string{
				meta.ReconcileRequestAnnotation: "2024-12-05T12:00:00Z",
			},
		},
		Spec: infrav1.TerraformSpec{
			Interval: metav1.Duration{Duration: 24 * time.Hour},
		},
		Status: infrav1.TerraformStatus{
			LastPlanAt:         &metav1.Time{Time: time.Now()},
			ObservedGeneration: 1,
		},
	}

	shouldReconcile, reason, requeueAfter := reconciler.shouldReconcile(tf, nil)
	g.Expect(shouldReconcile).To(BeTrue())
	g.Expect(reason).To(Equal("new reconcile request annotation present"))
	g.Expect(requeueAfter).To(Equal(time.Duration(0)))
}

func TestShouldReconcileWhenPendingPlan(t *testing.T) {
	Spec("This spec covers reconciling when there is a pending plan or an apply should run.")
	It("should return true without delay.")

	g := NewWithT(t)
	reconciler := &TerraformReconciler{}

	tf := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 1,
		},
		Spec: infrav1.TerraformSpec{
			Interval:    metav1.Duration{Duration: 24 * time.Hour},
			ApprovePlan: infrav1.ApprovePlanAutoValue,
		},
		Status: infrav1.TerraformStatus{
			LastPlanAt: &metav1.Time{Time: time.Now()},
			Plan: infrav1.PlanStatus{
				Pending: "pending-plan",
			},
		},
	}

	shouldReconcile, reason, requeueAfter := reconciler.shouldReconcile(tf, nil)
	g.Expect(shouldReconcile).To(BeTrue())
	g.Expect(reason).To(Equal("pending plan or apply should run"))
	g.Expect(requeueAfter).To(Equal(time.Duration(0)))
}

func TestShouldReconcileWhenNeverPlanned(t *testing.T) {
	Spec("This spec covers reconciling when no plan has been performed yet.")
	It("should return true without delay.")

	g := NewWithT(t)
	reconciler := &TerraformReconciler{}

	tf := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 1,
		},
		Spec: infrav1.TerraformSpec{
			Interval: metav1.Duration{Duration: 24 * time.Hour},
		},
	}

	shouldReconcile, reason, requeueAfter := reconciler.shouldReconcile(tf, nil)
	g.Expect(shouldReconcile).To(BeTrue())
	g.Expect(reason).To(Equal("never planned before"))
	g.Expect(requeueAfter).To(Equal(time.Duration(0)))
}

func TestShouldReconcileWhenDeleting(t *testing.T) {
	Spec("This spec covers reconciling while the object is being deleted.")
	It("should return true without delay.")

	g := NewWithT(t)
	reconciler := &TerraformReconciler{}

	now := metav1.Now()
	tf := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Generation:        1,
			DeletionTimestamp: &now,
		},
		Spec: infrav1.TerraformSpec{
			Interval: metav1.Duration{Duration: 24 * time.Hour},
		},
		Status: infrav1.TerraformStatus{
			LastPlanAt: &metav1.Time{Time: time.Now()},
		},
	}

	shouldReconcile, reason, requeueAfter := reconciler.shouldReconcile(tf, nil)
	g.Expect(shouldReconcile).To(BeTrue())
	g.Expect(reason).To(Equal("object is being deleted"))
	g.Expect(requeueAfter).To(Equal(time.Duration(0)))
}

func TestShouldReconcileWhenForceEnabled(t *testing.T) {
	Spec("This spec covers reconciling when force is enabled.")
	It("should return true without delay.")

	g := NewWithT(t)
	reconciler := &TerraformReconciler{}

	tf := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 1,
		},
		Spec: infrav1.TerraformSpec{
			Interval: metav1.Duration{Duration: 24 * time.Hour},
			Force:    true,
		},
		Status: infrav1.TerraformStatus{
			LastPlanAt: &metav1.Time{Time: time.Now()},
		},
	}

	shouldReconcile, reason, requeueAfter := reconciler.shouldReconcile(tf, nil)
	g.Expect(shouldReconcile).To(BeTrue())
	g.Expect(reason).To(Equal("force is enabled"))
	g.Expect(requeueAfter).To(Equal(time.Duration(0)))
}

func TestShouldReconcileWhenSourceRevisionChanges(t *testing.T) {
	Spec("This spec covers reconciling when the source revision has changed.")
	It("should return true without delay.")

	g := NewWithT(t)
	reconciler := &TerraformReconciler{}

	source := &sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "source",
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

	source.Status = sourcev1.GitRepositoryStatus{
		ObservedGeneration: int64(1),
		Conditions: []metav1.Condition{
			{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Time{Time: time.Now()},
				Reason:             "GitOperationSucceed",
				Message:            "Fetched revision: master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			},
		},
		Artifact: &meta.Artifact{
			Path:           "gitrepository/flux-system/source/b8e362c206e3d0cbb7ed22ced771a0056455a2fb.tar.gz",
			URL:            server.URL() + "/file.tar.gz",
			Revision:       "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
			Digest:         "sha256:80ddfd18eb96f7d31cadc1a8a5171c6e2d95df3f6c23b0ed9cd8dddf6dba1406",
			LastUpdateTime: metav1.Time{Time: time.Now()},
		},
	}

	tf := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 1,
		},
		Spec: infrav1.TerraformSpec{
			Interval: metav1.Duration{Duration: 24 * time.Hour},
		},
		Status: infrav1.TerraformStatus{
			LastPlanAt:          &metav1.Time{Time: time.Now()},
			LastPlannedRevision: "an-old-revision",
			LastAppliedRevision: "an-old-revision",
		},
	}

	shouldReconcile, reason, requeueAfter := reconciler.shouldReconcile(tf, source)
	g.Expect(shouldReconcile).To(BeTrue())
	g.Expect(reason).To(Equal("source revision has changed since last reconciliation attempt"))
	g.Expect(requeueAfter).To(Equal(time.Duration(0)))
}

func TestShouldReconcileWithRetryIntervalOnFailure(t *testing.T) {
	Spec("This spec covers reconciling once the retry interval has elapsed after a failed reconciliation.")
	It("should return true even if the main interval has not elapsed.")

	g := NewWithT(t)
	reconciler := &TerraformReconciler{}

	tf := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 1,
		},
		Spec: infrav1.TerraformSpec{
			Interval:      metav1.Duration{Duration: 24 * time.Hour},
			RetryInterval: &metav1.Duration{Duration: 30 * time.Minute},
		},
		Status: infrav1.TerraformStatus{
			LastPlanAt:             &metav1.Time{Time: time.Now().Add(-3 * time.Hour)},
			ObservedGeneration:     1,
			ReconciliationFailures: 1,
			Conditions: []metav1.Condition{
				{
					Message: "error running Apply: rpc error: code = Internal desc = exit status 1",
					Reason:  infrav1.TFExecApplyFailedReason,
					Type:    meta.ReadyCondition,
					Status:  metav1.ConditionFalse,
				},
				{
					Message: "Plan generated",
					Reason:  infrav1.PlannedWithChangesReason,
					Type:    infrav1.ConditionTypePlan,
					Status:  metav1.ConditionFalse,
				},
				{
					Message: "error running Apply: rpc error: code = Internal desc = exit status 1",
					Reason:  "TerraformAppliedFail",
					Type:    infrav1.ConditionTypeApply,
					Status:  metav1.ConditionFalse,
				},
			},
		},
	}

	shouldReconcile, reason, requeueAfter := reconciler.shouldReconcile(tf, nil)
	g.Expect(shouldReconcile).To(BeTrue())
	g.Expect(reason).To(Equal("retry interval has elapsed since last failed reconciliation"))
	g.Expect(requeueAfter).To(Equal(time.Duration(0)))
}

func TestShouldReconcileWaitsForRetryIntervalOnFailure(t *testing.T) {
	Spec("This spec covers deferring reconciliation until the retry interval elapses after a failure.")
	It("should return false with a requeue duration bounded by the retry interval.")

	g := NewWithT(t)
	reconciler := &TerraformReconciler{}

	tf := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Generation: 1,
		},
		Spec: infrav1.TerraformSpec{
			Interval:      metav1.Duration{Duration: 24 * time.Hour},
			RetryInterval: &metav1.Duration{Duration: 30 * time.Minute},
		},
		Status: infrav1.TerraformStatus{
			LastPlanAt:             &metav1.Time{Time: time.Now().Add(-10 * time.Minute)},
			ObservedGeneration:     1,
			ReconciliationFailures: 1,
			Conditions: []metav1.Condition{
				{
					Message: "error running Apply: rpc error: code = Internal desc = exit status 1",
					Reason:  infrav1.TFExecApplyFailedReason,
					Type:    meta.ReadyCondition,
					Status:  metav1.ConditionFalse,
				},
				{
					Message: "Plan generated",
					Reason:  infrav1.PlannedWithChangesReason,
					Type:    infrav1.ConditionTypePlan,
					Status:  metav1.ConditionFalse,
				},
				{
					Message: "error running Apply: rpc error: code = Internal desc = exit status 1",
					Reason:  "TerraformAppliedFail",
					Type:    infrav1.ConditionTypeApply,
					Status:  metav1.ConditionFalse,
				},
			},
		},
	}

	shouldReconcile, _, requeueAfter := reconciler.shouldReconcile(tf, nil)
	g.Expect(shouldReconcile).To(BeFalse())
	g.Expect(requeueAfter).To(BeNumerically(">", 19*time.Minute))
	g.Expect(requeueAfter).To(BeNumerically("<=", 20*time.Minute))
}
