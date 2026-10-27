package controllers

import (
	"fmt"
	"testing"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/apis/meta"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// secondArtifact is the second revision tc000053 switches to.
var secondArtifact = artifactFixture{"/2222.tar.gz", "master/ed22ced771a0056455a2fbb8e362c206e3d0cbb7", "sha256:525802635a47a5ae3f9c058a2b958aac0daef08efbe100a4fc16833df5201b94"}

// Test_000420_source_change_pending records the scenario below.
func Test_000420_source_change_pending(t *testing.T) {
	s := newScenario(t, "000420_source_change_pending")
	repo := s.gitRepo("char-000420-source", helloWorldArtifact)

	s.step("plan")
	s.terraform("char-000420-tf", fmt.Sprintf(`
path: ./terraform-hello-world-example
interval: 1h
retryInterval: 20s
sourceRef:
  kind: GitRepository
  name: %s
  namespace: flux-system
`, repo.Name))
	s.settle("plan pending is the first plan", func(p projection) bool { return p.Plan.Pending == "plan-master-b8e362c206" })

	s.step("source-change")
	s.setArtifact(repo, secondArtifact)
	s.settle("plan pending is the second plan", func(p projection) bool { return p.Plan.Pending == "plan-master-ed22ced771" })

	s.assertGolden()
}

// Test_000421_spec_change_pending records the scenario below.
// Like tc000054 it bumps the generation by changing spec.interval.
func Test_000421_spec_change_pending(t *testing.T) {
	s := newScenario(t, "000421_spec_change_pending")
	repo := s.gitRepo("char-000421-source", helloWorldArtifact)

	s.step("plan")
	s.terraform("char-000421-tf", fmt.Sprintf(`
path: ./terraform-hello-world-example
interval: 1h
retryInterval: 20s
sourceRef:
  kind: GitRepository
  name: %s
  namespace: flux-system
`, repo.Name))
	s.settle("plan pending set", func(p projection) bool { return p.Plan.Pending != "" && p.LastPlannedGeneration == 1 })

	s.step("spec-change")
	s.patch(func(tf *infrav1.Terraform) { tf.Spec.Interval = metav1.Duration{Duration: 2 * time.Hour} })
	generation := s.get().Generation
	s.settle("new lastPlannedGeneration", func(p projection) bool {
		return p.Plan.Pending != "" && p.LastPlannedGeneration == generation
	})

	s.assertGolden()
}

// Test_000422_apply_error_resets_plan records the scenario below.
// Like tc000170, a ConfigMap that already exists makes the apply fail.
func Test_000422_apply_error_resets_plan(t *testing.T) {
	s := newScenario(t, "000422_apply_error_resets_plan")
	const tf = "char-000422-tf"
	repo := s.gitRepo("char-000422-source", driftArtifact)

	s.cleanupDriftState(tf) // also removes the pre-existing ConfigMap
	pre := driftConfigMap(tf)
	pre.Data = map[string]string{"subject": "my plain cat"}
	s.g.Expect(k8sClient.Create(s.t.Context(), pre)).To(Succeed())

	s.step("fail")
	s.terraform(tf, driftSpec(t, tf, repo.Name, "approvePlan: auto\n", "", "20s"))
	s.settle("Apply False TerraformAppliedFail, pending cleared", func(p projection) bool {
		c := conditionOf(p, infrav1.ConditionTypeApply)
		return c.Status == metav1.ConditionFalse && c.Reason == "TerraformAppliedFail" && p.Plan.Pending == ""
	})

	s.step("recover")
	s.g.Expect(k8sClient.Delete(s.t.Context(), &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "cm-" + tf}})).To(Succeed())
	s.requestReconcile("char-000422-recover")
	s.settle("Apply True TerraformAppliedSucceed", func(p projection) bool {
		c := conditionOf(p, infrav1.ConditionTypeApply)
		return c.Status == metav1.ConditionTrue && c.Reason == infrav1.TFExecApplySucceedReason
	})

	s.assertGolden()
}

// badArtifact is the artifact tc000243 uses: a tarball the runner cannot unpack.
var badArtifact = artifactFixture{"/bad.tar.gz", "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb", "sha256:196d115c43583ccd10107d631d8a594be542a75911f9832a5ec2c1e22b65387b"}

// Test_000423_retry_limit records the scenario below.
// retryInterval is 5s, as in tc000243, so that reaching the limit takes seconds.
func Test_000423_retry_limit(t *testing.T) {
	s := newScenario(t, "000423_retry_limit")
	repo := s.gitRepo("char-000423-source", badArtifact)

	stalled := func(p projection) bool {
		c := conditionOf(p, meta.StalledCondition)
		return c.Status == metav1.ConditionTrue && c.Reason == infrav1.RetryLimitReachedReason
	}

	s.step("limit")
	s.terraform("char-000423-tf", fmt.Sprintf(`
approvePlan: auto
path: ./terraform-hello-world-example
remediation:
  retries: 3
interval: 1h
retryInterval: 5s
sourceRef:
  kind: GitRepository
  name: %s
  namespace: flux-system
`, repo.Name))
	s.settle("Stalled RetryLimitReached after 3 failures", func(p projection) bool {
		return stalled(p) && p.ReconciliationFailures == 3
	})

	s.step("stalled")
	s.requestReconcile("char-000423-stalled")
	s.settle("still Stalled after a reconcile", stalled)

	s.step("fewer-retries")
	s.patch(func(tf *infrav1.Terraform) { tf.Spec.Remediation = &infrav1.Remediation{Retries: 2} })
	s.settle("Stalled again after 2 failures", func(p projection) bool {
		return stalled(p) && p.ReconciliationFailures == 2
	})

	const newRevision = "master/aaabbbccc6e3d0cbb7ed22ced771a0056455a2fb"
	s.step("new-source")
	next := badArtifact
	next.Revision = newRevision
	s.setArtifact(repo, next)
	s.settle("new revision attempted and Stalled after 2 failures", func(p projection) bool {
		return stalled(p) && p.LastAttemptedRevision == newRevision && p.ReconciliationFailures == 2
	})

	s.assertGolden()
}

// Test_000424_webhook_fail records the scenario below.
// The webhooks and their routes are tc000340's (webhook_mock_test.go): the first
// passes, the second fails, the third is disabled.
func Test_000424_webhook_fail(t *testing.T) {
	s := newScenario(t, "000424_webhook_fail")
	repo := s.gitRepo("char-000424-source", helloWorldArtifact)

	s.step("plan")
	s.terraform("char-000424-tf", fmt.Sprintf(`
approvePlan: auto
path: ./terraform-hello-world-example
interval: 1h
retryInterval: 20s
sourceRef:
  kind: GitRepository
  name: %s
  namespace: flux-system
webhooks:
- stage: post-planning
  url: %s/terraform/admission/pass
  testExpression: "${{ .passed }}"
  errorMessageTemplate: "SHOULD PASS Violation: ${{ (index .violations 0).message }}"
- stage: post-planning
  url: %s/terraform/admission/fail
  testExpression: "${{ .passed }}"
  errorMessageTemplate: "SHOULD FAIL Violation: ${{ (index .violations 0).message }}"
- stage: post-planning
  enabled: false
  url: some-invalid-url
  testExpression: "${{ .passed }}"
  errorMessageTemplate: "SHOULD FAIL Violation: ${{ (index .violations 0).message }}"
`, repo.Name, server.URL(), server.URL()))
	s.settle("Plan False PostPlanningWebhookFailed", func(p projection) bool {
		c := conditionOf(p, infrav1.ConditionTypePlan)
		return c.Status == metav1.ConditionFalse && c.Reason == "PostPlanningWebhookFailed"
	})

	s.assertGolden()
}

// Test_000431_init_failed records the scenario below.
// The backend type does not exist, so `terraform init` fails without network
// access. The reason TFExecInitFailed is the one the branch planner reacts to,
// so it must not change.
func Test_000431_init_failed(t *testing.T) {
	s := newScenario(t, "000431_init_failed")
	const tf = "char-000431-tf"
	repo := s.gitRepo("char-000431-source", helloWorldArtifact)

	s.step("create")
	s.terraform(tf, hwSourceSpec("GitRepository", repo.Name, "flux-system", `backendConfig:
  customConfiguration: |
    backend "char-nonexistent" {}
`))
	// Deletion of an object whose init fails may not finish through the normal path.
	s.stripFinalizersOnCleanup()
	s.settle("Ready False TFExecInitFailed", func(p projection) bool {
		return readyIs(p, metav1.ConditionFalse, infrav1.TFExecInitFailedReason)
	})

	s.assertGolden()
}

// multiVarArtifact is the archive whose module declares the required variables
// region, environment and cluster_name (no defaults).
var multiVarArtifact = artifactFixture{"/tf-multi-var.tar.gz", "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb", "sha256:52fbbf10455df51136a0c43e0f548c01acdbafca5cbad12c787612e47a4aa815"}

// Test_000432_plan_failed records the scenario below.
// None of the required variables is supplied, so `terraform plan` fails.
func Test_000432_plan_failed(t *testing.T) {
	s := newScenario(t, "000432_plan_failed")
	const tf = "char-000432-tf"
	repo := s.gitRepo("char-000432-source", multiVarArtifact)

	s.step("create")
	s.terraform(tf, `
approvePlan: auto
path: ./tf-multi-var-with-outputs
interval: 1h
retryInterval: 20s
sourceRef:
  kind: GitRepository
  name: `+repo.Name+`
  namespace: flux-system
`)
	s.stripFinalizersOnCleanup()
	s.settle("Ready False TFExecPlanFailed", func(p projection) bool {
		return readyIs(p, metav1.ConditionFalse, infrav1.TFExecPlanFailedReason)
	})

	s.assertGolden()
}
