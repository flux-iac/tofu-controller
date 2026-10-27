package controllers

import (
	"fmt"
	"testing"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/apis/meta"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Test_000400_auto_apply_no_outputs records one auto-approved pass of the
// hello-world fixture, adapted from tc000010.
func Test_000400_auto_apply_no_outputs(t *testing.T) {
	s := newScenario(t, "000400_auto_apply_no_outputs")
	repo := s.gitRepo("char-000400-source", helloWorldArtifact)

	s.step("create")
	s.terraform("char-000400-auto-apply", fmt.Sprintf(`
approvePlan: auto
path: ./terraform-hello-world-example
interval: 1h
sourceRef:
  kind: GitRepository
  name: %s
  namespace: flux-system
`, repo.Name))
	s.settle("Ready True", func(p projection) bool {
		return conditionOf(p, meta.ReadyCondition).Status == metav1.ConditionTrue
	})

	s.assertGolden()
}

// Test_000401_auto_apply_outputs_written records the scenario below.
func Test_000401_auto_apply_outputs_written(t *testing.T) {
	s := newScenario(t, "000401_auto_apply_outputs_written")
	repo := s.gitRepo("char-000401-source", helloWorldArtifact)

	s.step("create")
	s.terraform("char-000401-tf", fmt.Sprintf(`
approvePlan: auto
writeOutputsToSecret:
  name: char-401-out
path: ./terraform-hello-world-example
interval: 1h
retryInterval: 20s
sourceRef:
  kind: GitRepository
  name: %s
  namespace: flux-system
`, repo.Name))
	s.settle("Ready True TerraformOutputsWritten", func(p projection) bool {
		c := conditionOf(p, meta.ReadyCondition)
		return c.Status == metav1.ConditionTrue && c.Reason == "TerraformOutputsWritten"
	})

	s.assertGolden()
}

// Test_000402_manual_approve_exact records the scenario below.
func Test_000402_manual_approve_exact(t *testing.T) {
	s := newScenario(t, "000402_manual_approve_exact")
	repo := s.gitRepo("char-000402-source", helloWorldArtifact)

	s.step("plan")
	s.terraform("char-000402-tf", fmt.Sprintf(`
path: ./terraform-hello-world-example
interval: 1h
retryInterval: 20s
sourceRef:
  kind: GitRepository
  name: %s
  namespace: flux-system
`, repo.Name))
	s.settle("plan pending set", func(p projection) bool { return p.Plan.Pending != "" })

	s.step("idle")
	s.patch(func(tf *infrav1.Terraform) {
		if tf.Annotations == nil {
			tf.Annotations = map[string]string{}
		}
		tf.Annotations[meta.ReconcileRequestAnnotation] = "char-000402-idle"
	})
	s.settle("pending unchanged", func(p projection) bool { return p.Plan.Pending != "" })

	s.step("approve")
	s.patch(func(tf *infrav1.Terraform) { tf.Spec.ApprovePlan = "plan-master-b8e362c206" })
	s.settle("Apply True", func(p projection) bool {
		return conditionOf(p, infrav1.ConditionTypeApply).Status == metav1.ConditionTrue
	})

	s.assertGolden()
}

// Test_000403_manual_approve_prefix records the scenario below.
func Test_000403_manual_approve_prefix(t *testing.T) {
	s := newScenario(t, "000403_manual_approve_prefix")
	repo := s.gitRepo("char-000403-source", helloWorldArtifact)

	s.step("plan")
	s.terraform("char-000403-tf", fmt.Sprintf(`
path: ./terraform-hello-world-example
interval: 1h
retryInterval: 20s
sourceRef:
  kind: GitRepository
  name: %s
  namespace: flux-system
`, repo.Name))
	s.settle("plan pending set", func(p projection) bool { return p.Plan.Pending != "" })

	s.step("approve")
	s.patch(func(tf *infrav1.Terraform) { tf.Spec.ApprovePlan = "plan-master-b8e3" })
	s.settle("Apply True", func(p projection) bool {
		return conditionOf(p, infrav1.ConditionTypeApply).Status == metav1.ConditionTrue
	})

	s.assertGolden()
}

// Test_000404_replan_via_approve records the scenario below.
func Test_000404_replan_via_approve(t *testing.T) {
	s := newScenario(t, "000404_replan_via_approve")
	repo := s.gitRepo("char-000404-source", helloWorldArtifact)

	s.step("plan")
	s.terraform("char-000404-tf", fmt.Sprintf(`
path: ./terraform-hello-world-example
interval: 1h
retryInterval: 20s
sourceRef:
  kind: GitRepository
  name: %s
  namespace: flux-system
`, repo.Name))
	s.settle("plan pending set", func(p projection) bool { return p.Plan.Pending != "" })

	s.step("approve")
	s.patch(func(tf *infrav1.Terraform) { tf.Spec.ApprovePlan = "replan-master-b8e362c206" })
	s.settle("plan pending set", func(p projection) bool {
		return p.Plan.Pending != "" && hasTime(p, "lastPlanAt")
	})

	s.assertGolden()
}

func hasTime(p projection, name string) bool {
	for _, n := range p.TimesSet {
		if n == name {
			return true
		}
	}
	return false
}

// Test_000405_external_replan records the scenario below.
func Test_000405_external_replan(t *testing.T) {
	s := newScenario(t, "000405_external_replan")
	repo := s.gitRepo("char-000405-source", helloWorldArtifact)

	s.step("plan")
	s.terraform("char-000405-tf", fmt.Sprintf(`
path: ./terraform-hello-world-example
interval: 1h
retryInterval: 20s
sourceRef:
  kind: GitRepository
  name: %s
  namespace: flux-system
`, repo.Name))
	s.settle("plan pending set", func(p projection) bool { return p.Plan.Pending != "" })

	s.step("replan")
	s.patchStatus(func(tf *infrav1.Terraform) {
		apimeta.SetStatusCondition(&tf.Status.Conditions, metav1.Condition{
			Type:    meta.ReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  "ReplanRequested",
			Message: "Replan requested",
		})
		tf.Status.Plan.Pending = ""
		tf.Status.LastPlannedRevision = ""
		tf.Status.LastAttemptedRevision = ""
	})
	s.patch(func(tf *infrav1.Terraform) {
		if tf.Annotations == nil {
			tf.Annotations = map[string]string{}
		}
		tf.Annotations[meta.ReconcileRequestAnnotation] = "char-000405-replan"
	})
	s.settle("plan pending set again", func(p projection) bool { return p.Plan.Pending != "" })

	s.assertGolden()
}

// Test_000406_plan_only records the scenario below.
func Test_000406_plan_only(t *testing.T) {
	s := newScenario(t, "000406_plan_only")
	repo := s.gitRepo("char-000406-source", helloWorldArtifact)

	s.step("create")
	s.terraform("char-000406-tf", fmt.Sprintf(`
approvePlan: auto
planOnly: true
path: ./terraform-hello-world-example
interval: 1h
retryInterval: 20s
sourceRef:
  kind: GitRepository
  name: %s
  namespace: flux-system
`, repo.Name))
	s.settle("Ready Unknown plan only", func(p projection) bool {
		c := conditionOf(p, meta.ReadyCondition)
		return c.Status == metav1.ConditionUnknown && c.Reason == infrav1.PlannedWithChangesReason
	})

	s.assertGolden()
}

// Test_000407_force records the scenario below.
func Test_000407_force(t *testing.T) {
	s := newScenario(t, "000407_force")
	repo := s.gitRepo("char-000407-source", helloWorldArtifact)

	s.step("create")
	s.terraform("char-000407-tf", fmt.Sprintf(`
force: true
path: ./terraform-hello-world-example
interval: 1h
retryInterval: 20s
sourceRef:
  kind: GitRepository
  name: %s
  namespace: flux-system
`, repo.Name))
	s.settle("Apply True", func(p projection) bool {
		return conditionOf(p, infrav1.ConditionTypeApply).Status == metav1.ConditionTrue
	})

	s.assertGolden()
}

// Test_000408_suspend records the scenario below.
func Test_000408_suspend(t *testing.T) {
	s := newScenario(t, "000408_suspend")
	repo := s.gitRepo("char-000408-source", helloWorldArtifact)

	s.step("suspended")
	s.terraform("char-000408-tf", fmt.Sprintf(`
approvePlan: auto
suspend: true
path: ./terraform-hello-world-example
interval: 1h
retryInterval: 20s
sourceRef:
  kind: GitRepository
  name: %s
  namespace: flux-system
`, repo.Name))
	s.settle("finalizer added while suspended", func(p projection) bool { return len(p.Finalizers) > 0 })

	s.step("resumed")
	s.patch(func(tf *infrav1.Terraform) { tf.Spec.Suspend = false })
	s.settle("Ready True", func(p projection) bool {
		return conditionOf(p, meta.ReadyCondition).Status == metav1.ConditionTrue
	})

	s.assertGolden()
}
