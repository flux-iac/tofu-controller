package controllers

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/mtls"
	"github.com/flux-iac/tofu-controller/runner"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestBreakGlassDoneErrorReturnsObject checks that a HasBreakTheGlassSessionDone
// error is returned with the object intact, so the generic failure path
// applies (failures +1, Warning ReconciliationFailed, retry) instead of a
// nil-object panic.
func TestBreakGlassDoneErrorReturnsObject(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("INSECURE_LOCAL_RUNNER", "1")
	t.Setenv("DISABLE_TF_K8S_BACKEND", "1")

	src := helloWorldSource(newArtifactServer(t))
	obj := fakeTerraform("break-glass-done-error")
	obj.Annotations = map[string]string{infrav1.BreakTheGlassAnnotation: "now"}
	c, _ := newFakePatcher(t, obj, src)

	r := newFakeReconciler(c)
	r.AllowBreakTheGlass = true
	ready := make(chan struct{})
	close(ready)
	r.CertRotator = &mtls.CertRotator{Ready: ready}
	fr := newFakeRunner()
	fr.errs["HasBreakTheGlassSessionDone"] = errors.New("runner went away")
	r.lookupOrCreateRunner = func(context.Context, *infrav1.Terraform, string) (runner.RunnerClient, func() error, error) {
		return fr, func() error { return nil }, nil
	}

	var seen *infrav1.Terraform
	r.observeReconcile = func(_ ctrl.Request, o *infrav1.Terraform, _ ctrl.Result, _ error, _ bool) {
		if o != nil {
			seen = o.DeepCopy()
		}
	}

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(obj)})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(seen).NotTo(BeNil(), "Reconcile lost the object")
	g.Expect(res).To(Equal(ctrl.Result{RequeueAfter: seen.GetRetryInterval()}))
	g.Expect(seen.Status.ReconciliationFailures).To(Equal(int64(1)))
	g.Expect(stored(t, c, obj).Status.ReconciliationFailures).To(Equal(int64(1)))

	recorded := drainEvents(r.EventRecorder.(*events.FakeRecorder))
	g.Expect(recorded).To(ContainElement("Warning " + infrav1.ReconciliationFailureReason + " runner went away"))
}

// TestBreakGlassPollingStopsOnContextCancel checks that cancelling the context
// during a session ends the polling with ctx.Err() without waiting out the
// 10s poll interval.
func TestBreakGlassPollingStopsOnContextCancel(t *testing.T) {
	g := NewWithT(t)
	r, s, obj, fr, _, _ := newLoggedPass(t, "break-glass-cancel")
	r.AllowBreakTheGlass = true
	fr.replies["HasBreakTheGlassSessionDone"] = &runner.BreakTheGlassReply{Success: false}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fr.onCall = func(name string) {
		if name == "HasBreakTheGlassSessionDone" {
			cancel()
		}
	}

	start := time.Now()
	err := r.breakGlass(ctx, s, obj)
	g.Expect(err).To(MatchError(context.Canceled))
	g.Expect(time.Since(start)).To(BeNumerically("<", 5*time.Second))
	g.Expect(fr.Calls()).To(Equal([]string{"StartBreakTheGlassSession", "HasBreakTheGlassSessionDone"}))
}

// drainEvents returns the events recorded so far.
func drainEvents(rec *events.FakeRecorder) []string {
	var out []string
	for {
		select {
		case e := <-rec.Events:
			out = append(out, strings.TrimSpace(e))
		default:
			return out
		}
	}
}
