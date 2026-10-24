package controllers

import (
	"fmt"
	"strings"
	"testing"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Test_000450_destroy records the scenario below.
// tc000130's flow: apply the configmap fixture, then patch spec.destroy to true.
// The plan is a destroy plan and is applied.
func Test_000450_destroy(t *testing.T) {
	s := newScenario(t, "000450_destroy")
	const tf = "char-000450-tf"
	repo := s.gitRepo("char-000450-source", driftArtifact)

	s.cleanupDriftState(tf)
	s.step("apply")
	s.terraform(tf, driftSpec(t, tf, repo.Name, "approvePlan: auto\n", "", "20s"))
	s.settle("Apply True TerraformAppliedSucceed", appliedOK)

	s.step("destroy")
	s.patch(func(o *infrav1.Terraform) { o.Spec.Destroy = true })
	s.settle("Apply True Destroy applied successfully", func(p projection) bool {
		return appliedOK(p) && conditionOf(p, infrav1.ConditionTypeApply).Message == "Destroy applied successfully"
	})

	s.assertGolden()
}

// Test_000451_destroy_nothing records the scenario below.
// tc000131's flow: apply with destroyResourcesOnDeletion, remove the managed
// ConfigMap and the state secret, stop auto-apply, then delete. The destroy plan
// finds nothing to destroy, so no destroy is applied.
func Test_000451_destroy_nothing(t *testing.T) {
	s := newScenario(t, "000451_destroy_nothing")
	const tf = "char-000451-tf"
	repo := s.gitRepo("char-000451-source", driftArtifact)

	s.cleanupDriftState(tf)
	s.step("apply")
	s.terraform(tf, driftSpec(t, tf, repo.Name, "approvePlan: auto\n", "destroyResourcesOnDeletion: true\n", "20s"))
	s.settle("Ready True", func(p projection) bool {
		return conditionOf(p, meta.ReadyCondition).Status == metav1.ConditionTrue
	})

	s.step("prepare")
	s.causeDrift(tf) // deletes the managed ConfigMap
	state := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: charNamespace, Name: "tfstate-default-" + tf}}
	s.g.Expect(k8sClient.Delete(t.Context(), state)).To(Succeed())
	s.patch(func(o *infrav1.Terraform) { o.Spec.ApprovePlan = "" })
	gen := s.get().Generation
	s.settle("replanned with changes pending (state and ConfigMap are gone)", func(p projection) bool {
		return p.Generation == gen && p.Plan.Pending != "" && hasTime(p, "lastPlanAt")
	})

	s.step("delete")
	s.delete()
	s.settle("Plan False No objects need to be destroyed", func(p projection) bool {
		c := conditionOf(p, infrav1.ConditionTypePlan)
		return c.Status == metav1.ConditionFalse && c.Message == "No objects need to be destroyed"
	})
	s.settleGone()

	s.assertGolden()
}

// Test_000452_delete records the scenario below.
// tc000120's flow: apply hello-world with an outputs secret, then delete it. The
// finalizer runs (no destroy), the object goes, and the pass ends there.
func Test_000452_delete(t *testing.T) {
	s := newScenario(t, "000452_delete")
	const tf = "char-000452-tf"
	repo := s.gitRepo("char-000452-source", helloWorldArtifact)

	s.cleanupDriftState(tf)
	s.step("apply")
	s.terraform(tf, hwSourceSpec("GitRepository", repo.Name, "flux-system",
		"writeOutputsToSecret:\n  name: "+tf+"-outputs\n  outputs:\n  - hello_world\n"))
	s.settle("Ready True TerraformOutputsWritten", func(p projection) bool {
		return readyIs(p, metav1.ConditionTrue, "TerraformOutputsWritten")
	})

	s.step("delete")
	s.delete()
	s.settleGone()

	s.assertGolden()
}

// Test_000453_destroy_on_delete records the scenario below.
// tc000121's flow: apply the configmap fixture with destroyResourcesOnDeletion,
// then delete. The finalizer plans and applies a destroy before the object goes.
func Test_000453_destroy_on_delete(t *testing.T) {
	s := newScenario(t, "000453_destroy_on_delete")
	const tf = "char-000453-tf"
	repo := s.gitRepo("char-000453-source", driftArtifact)

	s.cleanupDriftState(tf)
	s.step("apply")
	s.terraform(tf, driftSpec(t, tf, repo.Name, "approvePlan: auto\n", "destroyResourcesOnDeletion: true\n", "20s"))
	s.settle("Ready True", func(p projection) bool {
		return conditionOf(p, meta.ReadyCondition).Status == metav1.ConditionTrue
	})

	s.step("delete")
	s.delete()
	s.settleGone()

	s.assertGolden()
}

// Test_000454_delete_blocked_by_dependant records the scenario below.
// tc000350's shape with char-000454-* names. The recorded object is the dependency.
// A dependant (dependsOn the dependency) makes checkDependencies add the
// tf.dependency.of.<dependant> finalizer to it. Deleting the dependency is then
// blocked (Ready False DeletionBlockedByDependantsReason). Deleting the dependant
// removes that finalizer in its own finalize; the finalizer patch does not change
// the dependency's generation, so no event reconciles the dependency. Its
// retryInterval is 2h so that timer never fires within the test: once the
// dependant is gone, a reconcile request annotation is what lets the dependency
// finish deleting.
func Test_000454_delete_blocked_by_dependant(t *testing.T) {
	s := newScenario(t, "000454_delete_blocked_by_dependant")
	g := NewWithT(t)
	const (
		dependencyName = "char-000454-dependency"
		dependantName  = "char-000454-dependant"
	)
	dependencyRepo := s.gitRepo("char-000454-dependency-source", helloWorldArtifact)
	dependantRepo := s.gitRepo("char-000454-dependant-source", helloWorldArtifact)

	s.cleanupDriftState(dependencyName)
	s.cleanupDriftState(dependantName)
	s.step("apply")
	s.terraform(dependencyName, strings.Replace(hwSourceSpec("GitRepository", dependencyRepo.Name, "flux-system", ""),
		"retryInterval: 20s", "retryInterval: 2h", 1))
	s.stripFinalizersOnCleanup()
	s.settle("Ready True", func(p projection) bool {
		return conditionOf(p, meta.ReadyCondition).Status == metav1.ConditionTrue
	})

	By("creating the dependant and waiting for it to add its finalizer to the dependency")
	dependant := charTerraformObject(t, dependantName, hwSourceSpec("GitRepository", dependantRepo.Name, "flux-system",
		fmt.Sprintf("dependsOn:\n- name: %s\n", dependencyName)))
	g.Expect(k8sClient.Create(t.Context(), dependant)).To(Succeed())
	t.Cleanup(func() { waitResourceToBeDelete(NewWithT(t), dependant) })
	g.Eventually(func() []string { return s.get().Finalizers }, timeout*3, interval).
		Should(ContainElement(infrav1.TFDependencyOfPrefix + dependantName))
	s.quiet()

	s.step("blocked")
	s.delete()
	s.settle("Ready False DeletionBlockedByDependantsReason", func(p projection) bool {
		return readyIs(p, metav1.ConditionFalse, infrav1.DeletionBlockedByDependants)
	})

	s.step("unblocked")
	g.Expect(k8sClient.Delete(t.Context(), dependant)).To(Succeed())
	g.Eventually(func() bool {
		return apierrors.IsNotFound(k8sClient.Get(t.Context(), types.NamespacedName{Namespace: charNamespace, Name: dependantName}, &infrav1.Terraform{}))
	}, timeout*3, interval).Should(BeTrue())
	s.requestReconcile("char-000454-unblocked")
	s.settleGone()

	s.assertGolden()
}

// Test_000455_delete_suspended records the scenario below.
// tc000120's flow with spec.suspend patched to true before the delete. Deletion
// is handled before the suspend check, so the object finishes deleting: its
// secrets are finalized, but destroy and the pipeline do not run (no Plan or
// Apply milestones).
func Test_000455_delete_suspended(t *testing.T) {
	s := newScenario(t, "000455_delete_suspended")
	const tf = "char-000455-tf"
	repo := s.gitRepo("char-000455-source", helloWorldArtifact)

	s.cleanupDriftState(tf)
	s.step("apply")
	s.terraform(tf, hwSourceSpec("GitRepository", repo.Name, "flux-system", ""))
	// Safety net only: the object should finish deleting, and this is a no-op then.
	s.stripFinalizersOnCleanup()
	s.settle("Ready True", func(p projection) bool {
		return conditionOf(p, meta.ReadyCondition).Status == metav1.ConditionTrue
	})

	s.step("suspend")
	s.patch(func(o *infrav1.Terraform) { o.Spec.Suspend = true })
	suspendedGen := s.get().Generation
	s.settle("suspend patch reconciled", func(p projection) bool { return p.Generation == suspendedGen })

	s.step("delete")
	s.delete()
	s.settleGone()

	s.assertGolden()
}

// Test_000456_delete_missing_source records the scenario below.
// tc000120's flow: apply hello-world, delete its GitRepository, then delete the
// Terraform. Without destroyResourcesOnDeletion deletion does not need the
// source, so the missing source does not block it: the object finishes
// deleting, with no ArtifactFailed.
func Test_000456_delete_missing_source(t *testing.T) {
	s := newScenario(t, "000456_delete_missing_source")
	const tf = "char-000456-tf"
	repo := s.gitRepo("char-000456-source", helloWorldArtifact)

	s.cleanupDriftState(tf)
	s.step("apply")
	s.terraform(tf, hwSourceSpec("GitRepository", repo.Name, "flux-system", ""))
	// Safety net only: the object should finish deleting, and this is a no-op then.
	s.stripFinalizersOnCleanup()
	s.settle("Ready True", func(p projection) bool {
		return conditionOf(p, meta.ReadyCondition).Status == metav1.ConditionTrue
	})

	s.step("source-deleted")
	s.g.Expect(k8sClient.Delete(t.Context(), repo)).To(Succeed())
	s.g.Eventually(func() bool {
		return apierrors.IsNotFound(k8sClient.Get(t.Context(), client.ObjectKeyFromObject(repo), &sourcev1.GitRepository{}))
	}, timeout, interval).Should(BeTrue())
	// No settle: this segment may legitimately have no reconcile. quiet() lets the
	// manager's cached client see the deletion, and records any reconcile it triggers.
	s.quiet()

	s.step("delete")
	s.delete()
	s.settleGone()

	s.assertGolden()
}

// Test_000457_delete_missing_source_destroy records the scenario below.
// As 000456 with destroyResourcesOnDeletion: true. Destroy would need the source,
// and without it deletion stays blocked: Ready False ArtifactFailed. The
// source checks still apply when destroying. The cleanup removes the finalizers by hand.
func Test_000457_delete_missing_source_destroy(t *testing.T) {
	s := newScenario(t, "000457_delete_missing_source_destroy")
	const tf = "char-000457-tf"
	repo := s.gitRepo("char-000457-source", helloWorldArtifact)

	s.cleanupDriftState(tf)
	s.step("apply")
	s.terraform(tf, hwSourceSpec("GitRepository", repo.Name, "flux-system", "destroyResourcesOnDeletion: true\n"))
	// Destroy needs the source, so its absence blocks deletion; force-strip the object in cleanup.
	s.stripFinalizersOnCleanup()
	s.settle("Ready True", func(p projection) bool {
		return conditionOf(p, meta.ReadyCondition).Status == metav1.ConditionTrue
	})

	s.step("source-deleted")
	s.g.Expect(k8sClient.Delete(t.Context(), repo)).To(Succeed())
	s.g.Eventually(func() bool {
		return apierrors.IsNotFound(k8sClient.Get(t.Context(), client.ObjectKeyFromObject(repo), &sourcev1.GitRepository{}))
	}, timeout, interval).Should(BeTrue())
	// No settle: this segment may legitimately have no reconcile. quiet() lets the
	// manager's cached client see the deletion, and records any reconcile it triggers.
	s.quiet()

	s.step("delete")
	s.delete()
	s.settle("Ready False ArtifactFailed", func(p projection) bool {
		return readyIs(p, metav1.ConditionFalse, infrav1.ArtifactFailedReason)
	})
	s.expectStuck()

	s.assertGolden()
}

// Test_000458_backend_disabled_destroy records the scenario below: the destroy
// plan and apply with the backend disabled. A failing destroy cannot be produced
// in envtest. tc000010's hello-world with backendConfig.disable and destroy:
// true. The golden records what the reconciler does; local state is fresh on
// each pass.
func Test_000458_backend_disabled_destroy(t *testing.T) {
	s := newScenario(t, "000458_backend_disabled_destroy")
	const tf = "char-000458-tf"
	repo := s.gitRepo("char-000458-source", helloWorldArtifact)

	s.cleanupDriftState(tf)
	s.step("destroy")
	s.terraform(tf, hwSourceSpec("GitRepository", repo.Name, "flux-system", "backendConfig:\n  disable: true\ndestroy: true\n"))
	s.settle("Plan False No objects need to be destroyed", func(p projection) bool {
		c := conditionOf(p, infrav1.ConditionTypePlan)
		return c.Status == metav1.ConditionFalse && c.Message == "No objects need to be destroyed"
	})

	s.assertGolden()
}

// Test_000459_delete_missing_depends_on_target records the scenario below.
// The dependant dependsOn a Terraform that never exists, in its own namespace.
// Applying is blocked (DependencyNotReady). Deleting it skips
// checkDependencies, and finalize Gets every dependency to remove
// tf.dependency.of.<name>; a NotFound target is already released, so the
// finalizer is removed and the object goes.
func Test_000459_delete_missing_depends_on_target(t *testing.T) {
	s := newScenario(t, "000459_delete_missing_depends_on_target")
	const tf = "char-000459-tf"
	repo := s.gitRepo("char-000459-source", helloWorldArtifact)

	s.cleanupDriftState(tf)
	s.step("blocked")
	s.terraform(tf, fmt.Sprintf(`
approvePlan: auto
path: ./terraform-hello-world-example
interval: 1h
retryInterval: 2h
dependsOn:
- name: char-000459-never-created
sourceRef:
  kind: GitRepository
  name: %s
  namespace: flux-system
`, repo.Name))
	// Safety net only: the delete completes without it.
	s.stripFinalizersOnCleanup()
	s.settle("Ready False DependencyNotReady", func(p projection) bool {
		return readyIs(p, metav1.ConditionFalse, infrav1.DependencyNotReadyReason)
	})

	s.step("delete")
	s.delete()
	s.settleGone()

	s.assertGolden()
}
