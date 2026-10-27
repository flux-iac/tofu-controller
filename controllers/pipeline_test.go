package controllers

import (
	"context"
	"testing"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/mtls"
	"github.com/fluxcd/pkg/apis/meta"
	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestReconcileNormalStepOrder checks an auto-apply pass runs its steps in order:
// setup, plan, apply, outputs, then the working directory is
// cleaned up.
func TestReconcileNormalStepOrder(t *testing.T) {
	g := NewWithT(t)
	r, s, obj, fr, _, _ := newLoggedPass(t, "step-order")

	g.Expect(r.reconcileNormal(context.Background(), s, obj, helloWorldSource(newArtifactServer(t)))).To(Succeed())

	g.Expect(fr.Calls()).To(Equal([]string{
		// setup
		"UploadAndExtract", "WriteBackendConfig", "LookPath", "NewTerraform",
		"SetEnv", "GenerateVarsForTF", "GenerateTemplate", "Init", "SelectWorkspace",
		// plan
		"Plan", "SaveTFPlan",
		// apply
		"LoadTFPlan", "Apply",
		// outputs
		"Output",
		// deferred working-directory cleanup
		"CleanupDir",
	}))
}

// TestObserveReconcileSeesPatchedObject checks the test hook runs after the
// final deferred status patch and receives the object that patch wrote.
func TestObserveReconcileSeesPatchedObject(t *testing.T) {
	g := NewWithT(t)
	src := helloWorldSource(newArtifactServer(t))

	// A spec change (ObservedGeneration 0 != Generation 1) resets the
	// failures, then the pending plan waits for approval, so the
	// pass ends before the runner.
	obj := fakeTerraform("observe-patched")
	obj.Spec.ApprovePlan = ""
	obj.Annotations = map[string]string{meta.ReconcileRequestAnnotation: "requested-1"}
	obj.Status.ReconciliationFailures = 3
	obj.Status.LastAttemptedRevision = src.GetArtifact().Revision
	obj.Status.Plan.Pending = "plan-main-b8e362c206"

	c, _ := newFakePatcher(t, obj, src)
	r := newFakeReconciler(c)
	ready := make(chan struct{})
	close(ready)
	r.CertRotator = &mtls.CertRotator{Ready: ready}

	calls := 0
	var seen, storedAtHook *infrav1.Terraform
	r.observeReconcile = func(_ ctrl.Request, o *infrav1.Terraform, _ ctrl.Result, _ error, _ bool) {
		calls++
		seen = o.DeepCopy()
		storedAtHook = stored(t, c, o)
	}

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(obj)})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res).To(Equal(ctrl.Result{}))

	g.Expect(calls).To(Equal(1))
	g.Expect(seen.Status.ReconciliationFailures).To(BeZero())
	g.Expect(storedAtHook.Status.ReconciliationFailures).To(Equal(seen.Status.ReconciliationFailures))
	// Only the final deferred patch writes lastHandledReconcileAt, so the
	// hook ran after it.
	g.Expect(storedAtHook.Status.LastHandledReconcileAt).To(Equal("requested-1"))
	g.Expect(seen.Status.LastHandledReconcileAt).To(Equal("requested-1"))
}
