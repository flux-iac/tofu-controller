package controllers

import (
	"fmt"
	"testing"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/apis/meta"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Test_000440_source_missing_then_created records the scenario below.
// tc000191's missing source, driven through the reconciler: the Terraform is created
// first, pointing at a GitRepository that does not exist (Stalled, ArtifactFailed,
// retry requeue). The repository is then created with an artifact. The golden records
// whether the Stalled condition lingers once the source appears.
func Test_000440_source_missing_then_created(t *testing.T) {
	s := newScenario(t, "000440_source_missing_then_created")
	const sourceName = "char-000440-source"

	s.step("missing")
	tf := s.terraform("char-000440-tf", hwSourceSpec("GitRepository", sourceName, "flux-system", ""))
	s.settle("Stalled True ArtifactFailed", func(p projection) bool {
		c := conditionOf(p, meta.StalledCondition)
		return c.Status == metav1.ConditionTrue && c.Reason == "ArtifactFailed"
	})

	s.step("created")
	s.gitRepo(sourceName, helloWorldArtifact)
	// The repository is deleted before the Terraform unless this runs first: a
	// Terraform whose source is gone cannot be finalized.
	t.Cleanup(func() { waitResourceToBeDelete(s.g, tf) })
	s.settle("Ready True applied", func(p projection) bool {
		return readyIs(p, metav1.ConditionTrue, "TerraformAppliedSucceed") && appliedOK(p)
	})

	s.assertGolden()
}

// Test_000441_artifact_missing records the scenario below.
// tc000014's GitRepository without an artifact.
func Test_000441_artifact_missing(t *testing.T) {
	s := newScenario(t, "000441_artifact_missing")
	repo := s.gitRepoNoArtifact("char-000441-source")

	s.step("create")
	s.terraform("char-000441-tf", hwSourceSpec("GitRepository", repo.Name, "flux-system", ""))
	s.stripFinalizersOnCleanup()
	s.settle("Ready False ArtifactFailed", func(p projection) bool {
		return readyIs(p, metav1.ConditionFalse, infrav1.ArtifactFailedReason) &&
			conditionOf(p, "Ready").Message == "Source is not ready, artifact not found"
	})

	s.assertGolden()
}

// Test_000443_cross_ns_source records the scenario below.
// tc000015_source with NoCrossNamespaceRefs on: the source is named in another
// namespace. The Terraform sits in flux-system and the source in char-000443-other
// (the reverse of tc000015, which uses default and flux-system), because the harness
// creates Terraforms in flux-system. The check precedes the lookup, so the source
// does not have to exist.
func Test_000443_cross_ns_source(t *testing.T) {
	s := newScenario(t, "000443_cross_ns_source")
	denyCrossNamespaceRefs(t)
	s.otherNamespace("char-000443-other")

	s.step("create")
	s.terraform("char-000443-tf", hwSourceSpec("GitRepository", "char-000443-source", "char-000443-other", ""))
	s.stripFinalizersOnCleanup()
	s.settle("Stalled True AccessDenied", func(p projection) bool {
		c := conditionOf(p, meta.StalledCondition)
		return c.Status == metav1.ConditionTrue && c.Reason == "AccessDenied"
	})

	s.assertGolden()
}

// Test_000444_cross_ns_depends_on records the scenario below.
// tc000015_depends_on with NoCrossNamespaceRefs on: dependsOn names an object in
// another namespace. Stalled and Ready False AccessDenied, a Warning event,
// terminal result.
func Test_000444_cross_ns_depends_on(t *testing.T) {
	s := newScenario(t, "000444_cross_ns_depends_on")
	denyCrossNamespaceRefs(t)
	s.otherNamespace("char-000444-other")
	repo := s.gitRepo("char-000444-source", helloWorldArtifact)

	s.step("create")
	s.terraform("char-000444-tf", hwSourceSpec("GitRepository", repo.Name, "flux-system", `dependsOn:
- name: char-000444-dependency
  namespace: char-000444-other
`))
	// Safety net only: the dependsOn check is skipped while deleting, and finalize
	// treats the missing dependsOn target char-000444-dependency as released, so the
	// delete completes on its own.
	s.stripFinalizersOnCleanup()
	s.settle("Ready False AccessDenied", func(p projection) bool {
		return readyIs(p, metav1.ConditionFalse, infrav1.AccessDeniedReason)
	})

	s.assertGolden()
}

// Test_000445_cross_ns_cliconfig records the scenario below.
// tc000015_cliconfig with NoCrossNamespaceRefs on: cliConfigSecretRef names a
// secret in another namespace. Setup rejects it with Ready False AccessDenied.
func Test_000445_cross_ns_cliconfig(t *testing.T) {
	s := newScenario(t, "000445_cross_ns_cliconfig")
	denyCrossNamespaceRefs(t)
	s.otherNamespace("char-000445-other")
	repo := s.gitRepo("char-000445-source", helloWorldArtifact)

	s.step("create")
	s.terraform("char-000445-tf", hwSourceSpec("GitRepository", repo.Name, "flux-system", `cliConfigSecretRef:
  name: char-000445-cli
  namespace: char-000445-other
`))
	s.settle("Ready False AccessDenied", func(p projection) bool {
		return readyIs(p, metav1.ConditionFalse, infrav1.AccessDeniedReason)
	})

	s.assertGolden()
}

// advancedArtifact is tc000350's tf-hcl-values-advanced-example fixture, the
// dependant's source.
var advancedArtifact = artifactFixture{"/tf-hcl-values-advanced-example.tar.gz", "master/b8e362c206e3d0cbb7ed22ced771a0056455a2fb", "sha256:ff0f2a1576923ec52c3a77baa59357baf7985f97fb7cccafcbb76626de409b54"}

// Test_000446_depends_on records the scenario below.
// tc000350's shape with char-000446-* names: the dependency (manual approval, outputs
// to a secret) waits for approval, so the dependant, the object recorded, is blocked
// (DependencyNotReady, a Normal event). Approving the dependency makes it Ready. The
// dependant's retryInterval is 2h, so no timer drives it; it is reconciled by a
// reconcile request once the dependency is Ready and its outputs secret exists.
// checkDependencies adds the tf.dependency.of.<dependant> finalizer to the
// dependency; the dependant's projection does not show it, so the test asserts it
// on the dependency object.
func Test_000446_depends_on(t *testing.T) {
	s := newScenario(t, "000446_depends_on")
	g := NewWithT(t)
	const (
		dependencyName = "char-000446-dependency"
		dependantName  = "char-000446-dependant"
		planID         = "plan-master-b8e362c206"
	)
	dependencyRepo := s.gitRepo("char-000446-dependency-source", helloWorldArtifact)
	dependantRepo := s.gitRepo("char-000446-dependant-source", advancedArtifact)
	dependencyKey := types.NamespacedName{Namespace: charNamespace, Name: dependencyName}

	By("creating the dependency, and waiting until it is planned and waiting for approval")
	dependency := charTerraformObject(t, dependencyName, fmt.Sprintf(`
path: ./terraform-hello-world-example
interval: 1h
retryInterval: 20s
sourceRef:
  kind: GitRepository
  name: %s
  namespace: flux-system
writeOutputsToSecret:
  name: %s-outputs
`, dependencyRepo.Name, dependencyName))
	g.Expect(k8sClient.Create(t.Context(), dependency)).To(Succeed())
	t.Cleanup(func() { waitResourceToBeDelete(NewWithT(t), dependency) })
	g.Eventually(func() string {
		got := &infrav1.Terraform{}
		if err := k8sClient.Get(t.Context(), dependencyKey, got); err != nil {
			return ""
		}
		return got.Status.Plan.Pending
	}, timeout*3, interval).Should(Equal(planID))

	s.step("blocked")
	s.terraform(dependantName, fmt.Sprintf(`
values:
  cluster_name: my-cluster
  access_key: my-access-key
  secret_key: ${{ .secret_key.hello_world }}
dependsOn:
- name: %[1]s
readInputsFromSecrets:
- name: %[1]s-outputs
  as: secret_key
path: ./tf-hcl-values-advanced-example
sourceRef:
  kind: GitRepository
  name: %[2]s
  namespace: flux-system
interval: 1h
retryInterval: 2h
approvePlan: auto
writeOutputsToSecret:
  name: %[3]s-outputs
  outputs:
  - values_json_blob
`, dependencyName, dependantRepo.Name, dependantName))
	s.settle("Ready False DependencyNotReady", func(p projection) bool {
		return readyIs(p, metav1.ConditionFalse, infrav1.DependencyNotReadyReason)
	})

	s.step("ready")
	got := &infrav1.Terraform{}
	g.Expect(k8sClient.Get(t.Context(), dependencyKey, got)).To(Succeed())
	got.Spec.ApprovePlan = planID
	g.Expect(k8sClient.Update(t.Context(), got)).To(Succeed())
	By("waiting, outside the recording's settle, for the dependency to be Ready with its outputs written")
	g.Eventually(func() bool {
		dep := &infrav1.Terraform{}
		if err := k8sClient.Get(t.Context(), dependencyKey, dep); err != nil {
			return false
		}
		if dep.Generation != dep.Status.ObservedGeneration || !conditionsReadyTrue(dep) {
			return false
		}
		return k8sClient.Get(t.Context(), types.NamespacedName{Namespace: charNamespace, Name: dependencyName + "-outputs"}, &corev1.Secret{}) == nil
	}, timeout*3, interval).Should(BeTrue())
	s.requestReconcile("char-000446-ready")
	s.settle("Ready True TerraformOutputsWritten", func(p projection) bool {
		return readyIs(p, metav1.ConditionTrue, "TerraformOutputsWritten")
	})

	g.Expect(k8sClient.Get(t.Context(), dependencyKey, got)).To(Succeed())
	g.Expect(got.Finalizers).To(ContainElement(infrav1.TFDependencyOfPrefix + dependantName))

	s.assertGolden()
}

func conditionsReadyTrue(tf *infrav1.Terraform) bool {
	for _, c := range tf.Status.Conditions {
		if c.Type == meta.ReadyCondition {
			return c.Status == metav1.ConditionTrue
		}
	}
	return false
}
