package controllers

// Table tests pinning the pure reconcile decisions: stale-plan invalidation,
// the approval wait, backend selection, and whether to detect drift, plan,
// apply, run health checks and mark Ready. They record what the code does today.

import (
	"testing"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/apis/meta"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDecisionShouldPlan(t *testing.T) {
	r := &TerraformReconciler{}
	rows := []struct {
		name    string
		force   bool
		pending string
		want    bool
	}{
		{name: "force with pending", force: true, pending: "plan-main-abc", want: true},
		{name: "pending empty", pending: "", want: true},
		{name: "pending set", pending: "plan-main-abc", want: false},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			g := NewWithT(t)
			tf := &infrav1.Terraform{}
			tf.Spec.Force = row.force
			tf.Status.Plan.Pending = row.pending
			g.Expect(r.shouldPlan(tf)).To(Equal(row.want))
		})
	}
}

func TestDecisionShouldApply(t *testing.T) {
	r := &TerraformReconciler{}
	rows := []struct {
		name        string
		force       bool
		planOnly    bool
		approvePlan string
		pending     string
		want        bool
	}{
		{name: "force", force: true, want: true},
		{name: "planOnly with approvePlan auto and pending", planOnly: true, approvePlan: "auto", pending: "plan-master-b8e362c206", want: false},
		{name: "approvePlan empty", approvePlan: "", pending: "plan-master-b8e362c206", want: false},
		{name: "auto with pending", approvePlan: "auto", pending: "plan-master-b8e362c206", want: true},
		{name: "auto without pending", approvePlan: "auto", pending: "", want: false},
		{name: "exact id", approvePlan: "plan-master-b8e362c206", pending: "plan-master-b8e362c206", want: true},
		{name: "prefix of id", approvePlan: "plan-master-b8e3", pending: "plan-master-b8e362c206", want: true},
		{name: "unrelated id", approvePlan: "plan-other-1234567890", pending: "plan-master-b8e362c206", want: false},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			g := NewWithT(t)
			tf := &infrav1.Terraform{}
			tf.Spec.Force = row.force
			tf.Spec.PlanOnly = row.planOnly
			tf.Spec.ApprovePlan = row.approvePlan
			tf.Status.Plan.Pending = row.pending
			g.Expect(r.shouldApply(tf)).To(Equal(row.want))
		})
	}
}

func TestDecisionShouldDetectDrift(t *testing.T) {
	r := &TerraformReconciler{}
	const rev = "main@sha1:aaaa"
	// settled is a Terraform that has applied rev and has nothing pending.
	settled := func() *infrav1.Terraform {
		tf := &infrav1.Terraform{}
		tf.Generation = 1
		tf.Status.ObservedGeneration = 1
		tf.Status.LastAttemptedRevision = rev
		tf.Status.LastAppliedRevision = rev
		tf.Status.LastPlannedRevision = rev
		return tf
	}
	rows := []struct {
		name   string
		mutate func(tf *infrav1.Terraform)
		want   bool
	}{
		{name: "1 disableDriftDetection", mutate: func(tf *infrav1.Terraform) { tf.Spec.DisableDriftDetection = true }, want: false},
		{name: "2 destroy", mutate: func(tf *infrav1.Terraform) { tf.Spec.Destroy = true }, want: false},
		{name: "3 approvePlan disable", mutate: func(tf *infrav1.Terraform) {
			tf.Spec.ApprovePlan = infrav1.ApprovePlanDisableValue
			// even a brand-new object with a spec change detects drift
			tf.Status = infrav1.TerraformStatus{}
			tf.Generation = 2
		}, want: true},
		{name: "4 all three revisions empty", mutate: func(tf *infrav1.Terraform) {
			tf.Status.LastAttemptedRevision = ""
			tf.Status.LastAppliedRevision = ""
			tf.Status.LastPlannedRevision = ""
		}, want: false},
		{name: "5 generation differs from observedGeneration", mutate: func(tf *infrav1.Terraform) { tf.Generation = 2 }, want: false},
		{name: "6 attempted == applied == planned == revision, no pending", mutate: func(tf *infrav1.Terraform) {}, want: true},
		{name: "7 attempted == planned == revision, no pending (applied differs)", mutate: func(tf *infrav1.Terraform) {
			tf.Status.LastAppliedRevision = "main@sha1:old"
		}, want: true},
		{name: "fall-through: pending plan", mutate: func(tf *infrav1.Terraform) { tf.Status.Plan.Pending = "plan-main-aaaa" }, want: false},
		{name: "fall-through: revision moved", mutate: func(tf *infrav1.Terraform) { tf.Status.LastAttemptedRevision = "main@sha1:old" }, want: false},
		{name: "1 before 3: disableDriftDetection + approvePlan disable", mutate: func(tf *infrav1.Terraform) {
			tf.Spec.DisableDriftDetection = true
			tf.Spec.ApprovePlan = infrav1.ApprovePlanDisableValue
		}, want: false},
		{name: "2 before 3: destroy + approvePlan disable", mutate: func(tf *infrav1.Terraform) {
			tf.Spec.Destroy = true
			tf.Spec.ApprovePlan = infrav1.ApprovePlanDisableValue
		}, want: false},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			g := NewWithT(t)
			tf := settled()
			row.mutate(tf)
			g.Expect(r.shouldDetectDrift(tf, rev)).To(Equal(row.want))
		})
	}
}

func TestDecisionShouldDoHealthChecks(t *testing.T) {
	r := &TerraformReconciler{}
	checks := []infrav1.HealthCheck{{Name: "hc", Type: "tcp", Address: "localhost:80"}}
	cond := func(typ, reason string) metav1.Condition {
		return metav1.Condition{Type: typ, Status: metav1.ConditionTrue, Reason: reason}
	}
	rows := []struct {
		name       string
		checks     []infrav1.HealthCheck
		conditions []metav1.Condition
		want       bool
	}{
		{name: "no checks", checks: nil, conditions: []metav1.Condition{cond(infrav1.ConditionTypeApply, infrav1.TFExecApplySucceedReason)}, want: false},
		{name: "HealthCheck reason HealthChecksFailed", checks: checks, conditions: []metav1.Condition{cond(infrav1.ConditionTypeHealthCheck, infrav1.HealthChecksFailedReason)}, want: true},
		{name: "Apply succeeded and no HealthCheck", checks: checks, conditions: []metav1.Condition{cond(infrav1.ConditionTypeApply, infrav1.TFExecApplySucceedReason)}, want: true},
		{name: "Apply succeeded and HealthCheck succeeded", checks: checks, conditions: []metav1.Condition{
			cond(infrav1.ConditionTypeApply, infrav1.TFExecApplySucceedReason),
			cond(infrav1.ConditionTypeHealthCheck, "HealthChecksSucceed"),
		}, want: false},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			g := NewWithT(t)
			tf := &infrav1.Terraform{}
			tf.Spec.HealthChecks = row.checks
			tf.Status.Conditions = row.conditions
			g.Expect(r.shouldDoHealthChecks(tf)).To(Equal(row.want))
		})
	}
}

func TestDecisionClearStalePendingPlan(t *testing.T) {
	r := &TerraformReconciler{}
	const (
		oldRev  = "main@sha1:old"
		newRev  = "main@sha1:new"
		pending = "plan-main-b8e362c206"
	)
	rows := []struct {
		name                  string
		revision              string
		generation            int64
		lastPlannedGeneration int64
		approvePlan           string
		planOnly              bool
		force                 bool
		pending               string
		want                  bool
	}{
		// case 1 cannot be isolated: with a non-empty pending and !shouldApply it implies case 3.
		{name: "case 1 revision changed, replan approves pending", revision: newRev, generation: 1, lastPlannedGeneration: 1,
			approvePlan: "re" + pending, pending: pending, want: true},
		{name: "case 2 revision changed, planOnly with force (shouldApply true blocks case 3)", revision: newRev, generation: 1, lastPlannedGeneration: 1,
			force: true, planOnly: true, pending: pending, want: true},
		{name: "case 2 revision changed, planOnly, no pending (still returns true so the caller patches)", revision: newRev, generation: 1, lastPlannedGeneration: 1,
			planOnly: true, pending: "", want: true},
		{name: "case 3 revision changed, pending awaiting manual approval", revision: newRev, generation: 1, lastPlannedGeneration: 1,
			approvePlan: "", pending: pending, want: true},
		{name: "case 4 generation changed since plan, pending awaiting approval", revision: oldRev, generation: 2, lastPlannedGeneration: 1,
			approvePlan: "", pending: pending, want: true},
		{name: "revision unchanged and generation unchanged", revision: oldRev, generation: 1, lastPlannedGeneration: 1,
			approvePlan: "", pending: pending, want: false},
		{name: "case 3 blocked by shouldApply", revision: newRev, generation: 1, lastPlannedGeneration: 1,
			approvePlan: pending, pending: pending, want: false},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			g := NewWithT(t)
			tf := &infrav1.Terraform{}
			tf.Generation = row.generation
			tf.Spec.ApprovePlan = row.approvePlan
			tf.Spec.PlanOnly = row.planOnly
			tf.Spec.Force = row.force
			tf.Status.LastAttemptedRevision = oldRev
			tf.Status.LastPlannedGeneration = row.lastPlannedGeneration
			tf.Status.Plan.Pending = row.pending

			g.Expect(r.invalidateStalePlan(tf, row.revision)).To(Equal(row.want))
			if row.want {
				g.Expect(tf.Status.Plan.Pending).To(BeEmpty())
			} else {
				g.Expect(tf.Status.Plan.Pending).To(Equal(row.pending))
			}
		})
	}
}

func TestDecisionAwaitingManualApproval(t *testing.T) {
	r := &TerraformReconciler{}
	const pending = "plan-main-b8e362c206"
	rows := []struct {
		name        string
		approvePlan string
		pending     string
		want        bool
	}{
		{name: "pending, manual, not approved", approvePlan: "", pending: pending, want: true},
		{name: "auto", approvePlan: "auto", pending: pending, want: false},
		{name: "approved", approvePlan: pending, pending: pending, want: false},
		{name: "no pending", approvePlan: "", pending: "", want: false},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			g := NewWithT(t)
			tf := &infrav1.Terraform{}
			tf.Spec.ApprovePlan = row.approvePlan
			tf.Status.Plan.Pending = row.pending
			g.Expect(r.awaitingApproval(tf)).To(Equal(row.want))
		})
	}
}

func TestDecisionMarkReadyIfSettled(t *testing.T) {
	reconciling := metav1.Condition{Type: meta.ReconcilingCondition, Status: metav1.ConditionTrue, Reason: meta.ProgressingReason, Message: "Reconciliation in progress"}
	rows := []struct {
		name             string
		ready            metav1.Condition
		wantReady        bool
		reconcilingFirst bool // conditions are [Reconciling, Ready] instead of [Ready, Reconciling]
	}{
		{name: "Unknown TerraformAppliedSucceed", ready: metav1.Condition{Type: meta.ReadyCondition, Status: metav1.ConditionUnknown,
			Reason: infrav1.TFExecApplySucceedReason, Message: "Applied successfully: main@sha1:aaaa"}, wantReady: true},
		{name: "Unknown TerraformPlannedWithChanges Plan generated", ready: metav1.Condition{Type: meta.ReadyCondition, Status: metav1.ConditionUnknown,
			Reason: infrav1.PlannedWithChangesReason, Message: `Plan generated: set approvePlan: "plan-main-aaaa" to approve this plan.`}, wantReady: false},
		{name: "Unknown Progressing", ready: metav1.Condition{Type: meta.ReadyCondition, Status: metav1.ConditionUnknown,
			Reason: meta.ProgressingReason, Message: "Reconciliation in progress"}, wantReady: false},
		{name: "False", ready: metav1.Condition{Type: meta.ReadyCondition, Status: metav1.ConditionFalse,
			Reason: infrav1.TFExecApplyFailedReason, Message: "boom"}, wantReady: false},
		// Ready is set True by index before Reconciling is deleted, so the index
		// shift from the delete does not touch the wrong condition.
		{name: "Reconciling first, Unknown TerraformAppliedSucceed", ready: metav1.Condition{Type: meta.ReadyCondition, Status: metav1.ConditionUnknown,
			Reason: infrav1.TFExecApplySucceedReason, Message: "Applied successfully: main@sha1:aaaa"}, wantReady: true, reconcilingFirst: true},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			g := NewWithT(t)
			tf := &infrav1.Terraform{}
			tf.Status.Conditions = []metav1.Condition{row.ready, reconciling}
			if row.reconcilingFirst {
				tf.Status.Conditions = []metav1.Condition{reconciling, row.ready}
			}
			before := tf.DeepCopy()

			markReadyIfSettled(tf)

			if row.wantReady {
				want := row.ready
				want.Status = metav1.ConditionTrue
				g.Expect(tf.Status.Conditions).To(Equal([]metav1.Condition{want}))
			} else {
				g.Expect(tf).To(Equal(before))
			}
		})
	}
}

func TestDecisionBackendConfigFor(t *testing.T) {
	newTF := func() *infrav1.Terraform {
		tf := &infrav1.Terraform{}
		tf.Name = "my-tf"
		tf.Namespace = "flux-system"
		return tf
	}
	rows := []struct {
		name              string
		mutate            func(tf *infrav1.Terraform)
		disableK8SBackend bool
		wantHCL           func(tf *infrav1.Terraform) string
		wantWrite         bool
	}{
		{name: "cloud", mutate: func(tf *infrav1.Terraform) {
			tf.Spec.Cloud = &infrav1.CloudSpec{Organization: "org", Workspaces: &infrav1.CloudWorkspacesSpec{Name: "ws"}, Hostname: "app.terraform.io", Token: "tok"}
			// cloud wins over any backendConfig
			tf.Spec.BackendConfig = &infrav1.BackendConfigSpec{SecretSuffix: "ignored"}
		}, wantHCL: func(tf *infrav1.Terraform) string { return tf.Spec.Cloud.ToHCL() }, wantWrite: true},
		{name: "backendConfig.disable", mutate: func(tf *infrav1.Terraform) {
			tf.Spec.BackendConfig = &infrav1.BackendConfigSpec{Disable: true, CustomConfiguration: `backend "s3" {}`}
		}, wantHCL: func(*infrav1.Terraform) string { return "" }, wantWrite: false},
		{name: "custom configuration", mutate: func(tf *infrav1.Terraform) {
			tf.Spec.BackendConfig = &infrav1.BackendConfigSpec{CustomConfiguration: `backend "s3" {}`}
		}, wantHCL: func(*infrav1.Terraform) string {
			return "\nterraform {\n  backend \"s3\" {}\n}\n"
		}, wantWrite: true},
		{name: "backendConfig set", mutate: func(tf *infrav1.Terraform) {
			tf.Spec.BackendConfig = &infrav1.BackendConfigSpec{SecretSuffix: "suffix", InClusterConfig: true, ConfigPath: "/kube/config", Labels: map[string]string{"a": "b"}}
		}, wantHCL: func(*infrav1.Terraform) string {
			return `
terraform {
  backend "kubernetes" {
    secret_suffix     = "suffix"
    in_cluster_config = true
    config_path       = "/kube/config"
    namespace         = "flux-system"
    labels            = {
      "a" = "b"
    }
  }
}
`
		}, wantWrite: true},
		{name: "nil with disable flag", mutate: func(*infrav1.Terraform) {}, disableK8SBackend: true, wantHCL: func(*infrav1.Terraform) string {
			return "\nterraform {\n  backend \"local\" { }\n}"
		}, wantWrite: true},
		{name: "nil", mutate: func(*infrav1.Terraform) {}, wantHCL: func(*infrav1.Terraform) string {
			// getLabelsAsHCL(nil, 6) is "", leaving six spaces of indentation on the labels line.
			return "\nterraform {\n" +
				"  backend \"kubernetes\" {\n" +
				"    secret_suffix     = \"my-tf\"\n" +
				"    in_cluster_config = true\n" +
				"    namespace         = \"flux-system\"\n" +
				"    labels            = {\n" +
				"      \n" +
				"    }\n" +
				"  }\n" +
				"}\n"
		}, wantWrite: true},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			g := NewWithT(t)
			tf := newTF()
			row.mutate(tf)
			hcl, write := backendConfigFor(tf, row.disableK8SBackend)
			g.Expect(write).To(Equal(row.wantWrite))
			g.Expect(hcl).To(Equal(row.wantHCL(tf)))
		})
	}
}

// TestApprovalRecheckUsesPrefixRule checks the rule the pipeline re-checks a
// manual approval with after re-planning: the approved value equals the
// pending plan ID or is a prefix of it.
func TestApprovalRecheckUsesPrefixRule(t *testing.T) {
	rows := []struct {
		name     string
		approved string
		pending  string
		want     bool
	}{
		{name: "exact id", approved: "plan-master-b8e362c206", pending: "plan-master-b8e362c206", want: true},
		{name: "prefix of id", approved: "plan-master-b8e3", pending: "plan-master-b8e362c206", want: true},
		{name: "unrelated id", approved: "plan-other-1234567890", pending: "plan-master-b8e362c206", want: false},
		{name: "pending empty", approved: "plan-master-b8e3", pending: "", want: false},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			g := NewWithT(t)
			tf := &infrav1.Terraform{}
			tf.Spec.ApprovePlan = row.approved
			tf.Status.Plan.Pending = row.pending
			g.Expect((&TerraformReconciler{}).shouldApply(tf)).To(Equal(row.want))
		})
	}
}
