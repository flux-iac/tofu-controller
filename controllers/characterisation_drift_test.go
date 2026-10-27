package controllers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/apis/meta"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// driftArtifact is the tf-k8s-configmap fixture the drift source tests (tc000140,
// tc000150, tc000230) use, with the kubernetes provider version pinned: goldens
// compare event text that embeds the provider-rendered plan, so a new provider
// release must not change it. It creates a ConfigMap through the kubernetes provider.
var driftArtifact = artifactFixture{"/tf-k8s-configmap-pinned.tar.gz", "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb", "sha256:5e71e8bc91563ff03e7b9736178fc64c1cdbd2ab246329329fb205b340bf18f3"}

// driftKubeConfig copies the envtest kubeconfig into t.TempDir() and returns its
// path. The spec holds the path; the spec hash masks temp paths, so
// the golden's SpecHash stays stable. The temp dir outlives the Terraform object,
// because its cleanup is registered before s.terraform's.
func driftKubeConfig(t *testing.T) string {
	t.Helper()
	src, err := findKubeConfig(testEnv)
	if err != nil {
		t.Fatalf("finding kubeconfig: %v", err)
	}
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("reading kubeconfig: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "drift.kubecfg")
	if err := os.WriteFile(dst, b, 0o600); err != nil {
		t.Fatalf("writing kubeconfig: %v", err)
	}
	return dst
}

// driftSpec returns the spec of the drift fixture: the source tests' backendConfig
// and vars (kubeconfig, context, config_name), at interval 1h. approvePlan and
// extra are YAML fragments, each empty or ending in a newline.
func driftSpec(t *testing.T, tfName, repoName, approvePlan, extra, retryInterval string) string {
	t.Helper()
	kubeconfig := driftKubeConfig(t)
	return fmt.Sprintf(`
backendConfig:
  secretSuffix: %[1]s
  inClusterConfig: false
  configPath: %[3]s
%[4]s%[5]spath: ./tf-k8s-configmap
interval: 1h
retryInterval: %[6]s
sourceRef:
  kind: GitRepository
  name: %[2]s
  namespace: flux-system
vars:
- name: kubeconfig
  value: %[3]s
- name: context
  value: envtest
- name: config_name
  value: cm-%[1]s
`, tfName, repoName, kubeconfig, approvePlan, extra, retryInterval)
}

// driftConfigMap returns the ConfigMap the fixture manages for tfName.
func driftConfigMap(tfName string) *corev1.ConfigMap {
	return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "cm-" + tfName}}
}

// cleanupDriftState removes what the fixture leaves in the cluster: the managed
// ConfigMap and the terraform state and plan secrets. A later test with the same
// name would otherwise start from this test's state. Call it before s.terraform so
// that it runs after the Terraform object is gone.
func (s *scenario) cleanupDriftState(tfName string) {
	s.t.Cleanup(func() {
		ctx := context.Background() // t.Context() is already cancelled in cleanups
		_ = k8sClient.Delete(ctx, driftConfigMap(tfName))
		for _, prefix := range []string{"tfstate-default-", "tfplan-default-"} {
			_ = k8sClient.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: charNamespace, Name: prefix + tfName}})
		}
	})
}

// causeDrift deletes the managed ConfigMap, as the source tests do. The caller
// has already opened the segment for it.
func (s *scenario) causeDrift(tfName string) {
	s.t.Helper()
	cm := driftConfigMap(tfName)
	s.g.Eventually(func() error {
		return k8sClient.Get(s.t.Context(), client.ObjectKeyFromObject(cm), cm)
	}, timeout, interval).Should(Succeed(), "managed ConfigMap must exist before it is deleted")
	s.g.Expect(k8sClient.Delete(s.t.Context(), cm)).To(Succeed())
	s.g.Eventually(func() bool {
		return apierrors.IsNotFound(k8sClient.Get(s.t.Context(), types.NamespacedName{Namespace: "default", Name: "cm-" + tfName}, &corev1.ConfigMap{}))
	}, timeout, interval).Should(BeTrue())
}

// requestReconcile sets the reconcile-request annotation to value.
func (s *scenario) requestReconcile(value string) {
	s.t.Helper()
	s.patch(func(tf *infrav1.Terraform) {
		if tf.Annotations == nil {
			tf.Annotations = map[string]string{}
		}
		tf.Annotations[meta.ReconcileRequestAnnotation] = value
	})
}

// Test_000410_drift_detection_only records the scenario below.
func Test_000410_drift_detection_only(t *testing.T) {
	s := newScenario(t, "000410_drift_detection_only")
	const tf = "char-000410-tf"
	repo := s.gitRepo("char-000410-source", driftArtifact)

	s.cleanupDriftState(tf)
	s.step("apply")
	s.terraform(tf, driftSpec(t, tf, repo.Name, "approvePlan: auto\n", "", "20s"))
	s.settle("Ready True TerraformAppliedSucceed", func(p projection) bool {
		c := conditionOf(p, meta.ReadyCondition)
		return c.Status == metav1.ConditionTrue && c.Reason == infrav1.TFExecApplySucceedReason
	})

	s.step("detect")
	s.patch(func(tf *infrav1.Terraform) { tf.Spec.ApprovePlan = infrav1.ApprovePlanDisableValue })
	s.settle("Ready True NoDrift", func(p projection) bool {
		c := conditionOf(p, meta.ReadyCondition)
		return c.Status == metav1.ConditionTrue && c.Reason == infrav1.NoDriftReason
	})

	s.step("drift")
	s.causeDrift(tf)
	s.requestReconcile("char-000410-drift")
	s.settle("Ready False DriftDetected", func(p projection) bool {
		c := conditionOf(p, meta.ReadyCondition)
		return c.Status == metav1.ConditionFalse && c.Reason == infrav1.DriftDetectedReason
	})

	s.assertGolden()
}

// Test_000411_drift_auto records the scenario below.
func Test_000411_drift_auto(t *testing.T) {
	s := newScenario(t, "000411_drift_auto")
	const tf = "char-000411-tf"
	repo := s.gitRepo("char-000411-source", driftArtifact)

	s.cleanupDriftState(tf)
	s.step("apply")
	s.terraform(tf, driftSpec(t, tf, repo.Name, "approvePlan: auto\n", "", "20s"))
	s.settle("Ready True TerraformAppliedSucceed", func(p projection) bool {
		c := conditionOf(p, meta.ReadyCondition)
		return c.Status == metav1.ConditionTrue && c.Reason == infrav1.TFExecApplySucceedReason
	})

	s.step("drift")
	s.causeDrift(tf)
	s.requestReconcile("char-000411-drift")
	s.settle("lastAppliedByDriftDetectionAt set", func(p projection) bool {
		return hasTime(p, "lastAppliedByDriftDetectionAt")
	})

	s.assertGolden()
}

// Test_000412_drift_manual records the scenario below.
func Test_000412_drift_manual(t *testing.T) {
	s := newScenario(t, "000412_drift_manual")
	const tf = "char-000412-tf"
	repo := s.gitRepo("char-000412-source", driftArtifact)

	s.cleanupDriftState(tf)
	s.step("plan")
	s.terraform(tf, driftSpec(t, tf, repo.Name, "", "", "20s"))
	s.settle("plan pending set", func(p projection) bool { return p.Plan.Pending != "" })

	s.step("apply")
	s.patch(func(tf *infrav1.Terraform) { tf.Spec.ApprovePlan = "plan-master-b8e362c206" })
	s.settle("Apply True", func(p projection) bool {
		return conditionOf(p, infrav1.ConditionTypeApply).Status == metav1.ConditionTrue
	})

	s.step("drift")
	s.causeDrift(tf)
	s.requestReconcile("char-000412-drift")
	s.settle("Ready False DriftDetected", func(p projection) bool {
		c := conditionOf(p, meta.ReadyCondition)
		return c.Status == metav1.ConditionFalse && c.Reason == infrav1.DriftDetectedReason
	})

	s.assertGolden()
}

// Test_000413_drift_with_retry_limit records the scenario below.
// retryInterval is 2h: not equal to interval, so results record as "retry", and
// long enough that only the annotations drive reconciles.
func Test_000413_drift_with_retry_limit(t *testing.T) {
	s := newScenario(t, "000413_drift_with_retry_limit")
	const tf = "char-000413-tf"
	repo := s.gitRepo("char-000413-source", driftArtifact)

	s.cleanupDriftState(tf)
	s.step("plan")
	s.terraform(tf, driftSpec(t, tf, repo.Name, "", "remediation:\n  retries: 2\n", "2h"))
	s.settle("plan pending set", func(p projection) bool { return p.Plan.Pending != "" })

	s.step("apply")
	s.patch(func(tf *infrav1.Terraform) { tf.Spec.ApprovePlan = "plan-master-b8e362c206" })
	s.settle("Apply True", func(p projection) bool {
		return conditionOf(p, infrav1.ConditionTypeApply).Status == metav1.ConditionTrue
	})

	// Drift is not a failure: each segment settles on DriftDetected with
	// its reconcile request handled, and the failure count never moves.
	for n := 1; n <= 3; n++ {
		name := fmt.Sprintf("char-000413-drift-%d", n)
		s.step(fmt.Sprintf("drift-%d", n))
		if n == 1 {
			s.causeDrift(tf)
		}
		s.requestReconcile(name)
		s.settle("Ready False DriftDetected, "+name+" handled", func(p projection) bool {
			c := conditionOf(p, meta.ReadyCondition)
			return c.Status == metav1.ConditionFalse && c.Reason == infrav1.DriftDetectedReason &&
				p.LastHandledReconcileAt == name
		})
	}

	s.assertGolden()
}
