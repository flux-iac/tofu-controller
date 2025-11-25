package controllers

import (
	"testing"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestShouldReconcileSkipsWhenIntervalNotElapsed(t *testing.T) {
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

	shouldReconcile, requeueAfter := reconciler.shouldReconcile(tf)
	g.Expect(shouldReconcile).To(BeFalse())
	g.Expect(requeueAfter).To(BeNumerically(">", 17*time.Hour))
	g.Expect(requeueAfter).To(BeNumerically("<=", 24*time.Hour))
}

func TestShouldReconcileWhenIntervalElapsed(t *testing.T) {
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

	shouldReconcile, requeueAfter := reconciler.shouldReconcile(tf)
	g.Expect(shouldReconcile).To(BeTrue())
	g.Expect(requeueAfter).To(Equal(time.Duration(0)))
}

func TestShouldReconcileWhenGenerationChanged(t *testing.T) {
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

	shouldReconcile, requeueAfter := reconciler.shouldReconcile(tf)
	g.Expect(shouldReconcile).To(BeTrue())
	g.Expect(requeueAfter).To(Equal(time.Duration(0)))
}

func TestShouldReconcileWhenPendingPlan(t *testing.T) {
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

	shouldReconcile, requeueAfter := reconciler.shouldReconcile(tf)
	g.Expect(shouldReconcile).To(BeTrue())
	g.Expect(requeueAfter).To(Equal(time.Duration(0)))
}

func TestShouldReconcileWhenNeverPlanned(t *testing.T) {
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

	shouldReconcile, requeueAfter := reconciler.shouldReconcile(tf)
	g.Expect(shouldReconcile).To(BeTrue())
	g.Expect(requeueAfter).To(Equal(time.Duration(0)))
}

func TestShouldReconcileWhenDeleting(t *testing.T) {
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

	shouldReconcile, requeueAfter := reconciler.shouldReconcile(tf)
	g.Expect(shouldReconcile).To(BeTrue())
	g.Expect(requeueAfter).To(Equal(time.Duration(0)))
}

func TestShouldReconcileWhenForceEnabled(t *testing.T) {
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

	shouldReconcile, requeueAfter := reconciler.shouldReconcile(tf)
	g.Expect(shouldReconcile).To(BeTrue())
	g.Expect(requeueAfter).To(Equal(time.Duration(0)))
}
