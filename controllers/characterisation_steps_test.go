package controllers

import (
	"context"
	"fmt"
	"testing"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/apis/meta"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// forceUnlockLockID is the lock holder tc000290 plants in the state lease.
const forceUnlockLockID = "f2ab685b-f84d-ac0b-a125-378a22877e8d"

// cleanupLease removes the state lease the kubernetes backend leaves behind. Call
// it before s.terraform, as with cleanupDriftState.
func (s *scenario) cleanupLease(tfName string) {
	s.t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), &coordinationv1.Lease{ObjectMeta: metav1.ObjectMeta{Namespace: charNamespace, Name: "lock-tfstate-default-" + tfName}})
	})
}

// holdLock makes the state lease look held by forceUnlockLockID, as tc000290 does.
func (s *scenario) holdLock(tfName string) {
	s.t.Helper()
	var lease coordinationv1.Lease
	key := types.NamespacedName{Namespace: charNamespace, Name: "lock-tfstate-default-" + tfName}
	s.g.Expect(k8sClient.Get(s.t.Context(), key, &lease)).To(Succeed())
	base := client.MergeFrom(lease.DeepCopy())
	id := forceUnlockLockID
	lease.Annotations = map[string]string{
		"app.terraform.io/lock-info": fmt.Sprintf(`{"ID": "%s","Operation": "OperationTypeApply", "Info": "", "Who":"%s", "Version": "1.2.7", "Created": "2022-08-17T15:44:07.0Z", "Path": ""}`, id, tfName),
	}
	lease.Spec.HolderIdentity = &id
	s.g.Expect(k8sClient.Patch(s.t.Context(), &lease, base)).To(Succeed())
}

func appliedOK(p projection) bool {
	c := conditionOf(p, infrav1.ConditionTypeApply)
	return c.Status == metav1.ConditionTrue && c.Reason == infrav1.TFExecApplySucceedReason
}

// Test_000425_force_unlock_yes records the scenario below.
// tc000290's forceUnlock: yes path: apply, hold the lock, then set forceUnlock yes.
func Test_000425_force_unlock_yes(t *testing.T) {
	s := newScenario(t, "000425_force_unlock_yes")
	const tf = "char-000425-tf"
	repo := s.gitRepo("char-000425-source", driftArtifact)

	s.cleanupDriftState(tf)
	s.cleanupLease(tf)
	s.step("apply")
	s.terraform(tf, driftSpec(t, tf, repo.Name, "approvePlan: auto\n", "", "20s"))
	s.settle("Apply True TerraformAppliedSucceed", appliedOK)

	s.step("lock")
	s.holdLock(tf)
	s.requestReconcile("char-000425-lock")
	s.settle("StateLocked True with the lock identifier pending", func(p projection) bool {
		c := conditionOf(p, infrav1.ConditionTypeStateLocked)
		return c.Status == metav1.ConditionTrue && p.Lock.Pending == forceUnlockLockID
	})

	s.step("unlock")
	s.patch(func(tf *infrav1.Terraform) {
		tf.Spec.TFState = &infrav1.TFStateSpec{ForceUnlock: infrav1.ForceUnlockEnumYes, LockIdentifier: forceUnlockLockID}
	})
	s.settle("StateLocked removed", func(p projection) bool {
		c := conditionOf(p, infrav1.ConditionTypeStateLocked)
		return c.Type == "" && p.Lock.Pending == ""
	})

	s.assertGolden()
}

// Test_000426_force_unlock_auto records the scenario below.
// tc000290's forceUnlock: auto path: the spec asks for auto unlock from the start,
// then the lock is held.
func Test_000426_force_unlock_auto(t *testing.T) {
	s := newScenario(t, "000426_force_unlock_auto")
	const tf = "char-000426-tf"
	repo := s.gitRepo("char-000426-source", driftArtifact)

	s.cleanupDriftState(tf)
	s.cleanupLease(tf)
	s.step("apply")
	s.terraform(tf, driftSpec(t, tf, repo.Name, "approvePlan: auto\n", "tfstate:\n  forceUnlock: auto\n", "20s"))
	s.settle("Apply True TerraformAppliedSucceed", appliedOK)

	s.step("lock")
	s.holdLock(tf)
	s.requestReconcile("char-000426-lock")
	s.settle("StateLocked removed, Ready True NoDrift", func(p projection) bool {
		c := conditionOf(p, meta.ReadyCondition)
		return conditionOf(p, infrav1.ConditionTypeStateLocked).Type == "" && c.Status == metav1.ConditionTrue && c.Reason == infrav1.NoDriftReason
	})

	s.assertGolden()
}

// healthArtifact is the tf-health-check-example fixture tc000240 and tc000242 use.
// It has three real outputs (foo, port, bar), which the hello-world fixture of
// tc000040, tc000041 and tc000043 (one output) cannot offer.
var healthArtifact = artifactFixture{"/tf-health-check.tar.gz", "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb", "sha256:84b1410341b0e87d811bb1b812741e84a74ea00db851f88fd0855589b5093d14"}

// healthSpec returns the spec of the health-check fixture with auto approval, at
// interval 1h and retryInterval 20s. Like tc000242 it keeps the state in the
// kubernetes backend, so that a later reconcile sees no drift. extra is a YAML
// fragment, empty or ending in a newline.
func healthSpec(t *testing.T, tfName, repoName, extra string) string {
	t.Helper()
	return fmt.Sprintf(`
approvePlan: auto
backendConfig:
  secretSuffix: %[1]s
  inClusterConfig: false
  configPath: %[3]s
%[4]spath: ./tf-health-check-example
interval: 1h
retryInterval: 20s
sourceRef:
  kind: GitRepository
  name: %[2]s
  namespace: flux-system
`, tfName, repoName, driftKubeConfig(t), extra)
}

// Test_000427_outputs_alias_unsorted records the scenario below.
//
// The steady segment does not end in Ready NoDrift: the alias makes the keys in
// the secret never equal the keys in the spec, so outputs always count as drifted.
// outputsMayBeDrifted sorts a copy, so generation and SpecHash do not change.
// The outputs are listed unsorted, one aliased. The output secret is created first, holding exactly what the
// reconciler would write: the "Outputs written" event lists the written keys in
// map order, so a first write would make the golden nondeterministic.
func Test_000427_outputs_alias_unsorted(t *testing.T) {
	s := newScenario(t, "000427_outputs_alias_unsorted")
	const tf, secret = "char-000427-tf", "char-000427-out"
	repo := s.gitRepo("char-000427-source", healthArtifact)

	out := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: charNamespace, Name: secret},
		Data:       map[string][]byte{"a_alias": []byte("google.com"), "port": []byte("80")},
	}
	s.g.Expect(k8sClient.Create(s.t.Context(), out)).To(Succeed())
	s.t.Cleanup(func() { waitResourceToBeDelete(NewWithT(s.t), out) })

	s.cleanupDriftState(tf) // tfstate and tfplan secrets
	s.step("apply")
	s.terraform(tf, healthSpec(t, tf, repo.Name, fmt.Sprintf(`writeOutputsToSecret:
  name: %s
  outputs:
  - port
  - foo:a_alias
`, secret)))
	s.settle("Ready True TerraformOutputsWritten", func(p projection) bool {
		c := conditionOf(p, meta.ReadyCondition)
		return c.Status == metav1.ConditionTrue && c.Reason == "TerraformOutputsWritten"
	})

	s.step("steady")
	s.requestReconcile("char-000427-steady")
	s.settle("steady request handled, Ready True TerraformOutputsWritten, spec untouched", func(p projection) bool {
		c := conditionOf(p, meta.ReadyCondition)
		return c.Status == metav1.ConditionTrue && c.Reason == "TerraformOutputsWritten" &&
			p.LastHandledReconcileAt == "char-000427-steady" &&
			p.Generation == 1 && p.ObservedGeneration == 1
	})

	s.assertGolden()
}

// Test_000428_inventory_missing records the scenario below.
// tc000280's fixture with enableInventory: the inventory is cleared from the
// status, then a reconcile is requested. The annotation alone does not refill it:
// drift detection finds no drift and returns before the inventory step. A
// spec change skips drift detection, so that reconcile reaches the inventory
// step.
func Test_000428_inventory_missing(t *testing.T) {
	s := newScenario(t, "000428_inventory_missing")
	const tf = "char-000428-tf"
	repo := s.gitRepo("char-000428-source", driftArtifact)

	s.cleanupDriftState(tf)
	s.step("apply")
	s.terraform(tf, driftSpec(t, tf, repo.Name, "approvePlan: auto\n", "enableInventory: true\n", "20s"))
	s.settle("inventory entries recorded", func(p projection) bool { return p.InventoryEntries > 0 && appliedOK(p) })

	s.step("annotate")
	s.patchStatus(func(tf *infrav1.Terraform) { tf.Status.Inventory = nil })
	s.requestReconcile("char-000428-annotate")
	s.settle("inventory still missing, Ready True NoDrift", func(p projection) bool {
		return p.InventoryEntries == -1 && conditionOf(p, meta.ReadyCondition).Reason == infrav1.NoDriftReason
	})

	s.step("refill")
	s.patch(func(tf *infrav1.Terraform) { tf.Spec.Interval = metav1.Duration{Duration: 2 * time.Hour} })
	s.settle("inventory entries recorded again", func(p projection) bool { return p.InventoryEntries > 0 })

	s.assertGolden()
}

// Test_000429_health_pass records the scenario below.
// The HTTP health check hits the /get route that tc000240 uses.
func Test_000429_health_pass(t *testing.T) {
	s := newScenario(t, "000429_health_pass")
	const tf = "char-000429-tf"
	repo := s.gitRepo("char-000429-source", healthArtifact)

	s.cleanupDriftState(tf) // tfstate and tfplan secrets
	s.step("apply")
	s.terraform(tf, healthSpec(t, tf, repo.Name, fmt.Sprintf(`healthChecks:
- name: httpTestGood
  type: http
  url: %s/get
  timeout: 5s
`, server.URL())))
	s.settle("HealthCheck True", func(p projection) bool {
		return conditionOf(p, infrav1.ConditionTypeHealthCheck).Status == metav1.ConditionTrue
	})

	s.assertGolden()
}

// Test_000430_health_fail records the scenario below.
// The HTTP health check hits /something-else, a route the mock server does not
// serve, as tc000242 does.
func Test_000430_health_fail(t *testing.T) {
	s := newScenario(t, "000430_health_fail")
	const tf = "char-000430-tf"
	repo := s.gitRepo("char-000430-source", healthArtifact)

	s.cleanupDriftState(tf) // tfstate and tfplan secrets
	s.step("apply")
	s.terraform(tf, healthSpec(t, tf, repo.Name, fmt.Sprintf(`healthChecks:
- name: httpTestBadUrl
  type: http
  url: %s/something-else
  timeout: 5s
`, server.URL())))
	s.settle("HealthCheck False", func(p projection) bool {
		return conditionOf(p, infrav1.ConditionTypeHealthCheck).Status == metav1.ConditionFalse
	})

	s.assertGolden()
}
