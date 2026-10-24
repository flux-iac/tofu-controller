package controllers

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/mtls"
	"github.com/flux-iac/tofu-controller/runner"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/go-logr/logr/funcr"
	. "github.com/onsi/gomega"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// patchLog is one ordered log of the runner RPCs a pass makes and the status
// patches it writes, with a copy of the stored object after each patch.
type patchLog struct {
	mu      sync.Mutex
	entries []string
	patched []*infrav1.Terraform
}

func (l *patchLog) add(entry string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, entry)
}

func (l *patchLog) Entries() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.entries...)
}

func (l *patchLog) Patched() []*infrav1.Terraform {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]*infrav1.Terraform(nil), l.patched...)
}

// interceptor returns funcs that log "patch:<Ready message>" after each
// successful status patch, keeping the stored object.
func (l *patchLog) interceptor() interceptor.Funcs {
	return interceptor.Funcs{
		SubResourcePatch: func(ctx context.Context, c client.Client, sub string, obj client.Object, p client.Patch, opts ...client.SubResourcePatchOption) error {
			if err := c.SubResource(sub).Patch(ctx, obj, p, opts...); err != nil {
				return err
			}
			tf := obj.(*infrav1.Terraform).DeepCopy()
			msg := ""
			if ready := apimeta.FindStatusCondition(tf.Status.Conditions, meta.ReadyCondition); ready != nil {
				msg = ready.Message
			}
			l.mu.Lock()
			defer l.mu.Unlock()
			l.entries = append(l.entries, "patch:"+msg)
			l.patched = append(l.patched, tf)
			return nil
		},
	}
}

// newLoggedPass returns a reconciler, session and object for one pipeline
// pass against the fake runner, with RPCs and status patches in one log.
func newLoggedPass(t *testing.T, name string) (*TerraformReconciler, *runnerSession, *infrav1.Terraform, *fakeRunner, *patchLog, client.Client) {
	t.Helper()
	t.Setenv("DISABLE_TF_K8S_BACKEND", "1")

	log := &patchLog{}
	obj := fakeTerraform(name)
	c, patcher := newInterceptedFakePatcher(t, log.interceptor(), obj)
	r := newFakeReconciler(c)
	src := helloWorldSource(newArtifactServer(t))
	fr := newFakeRunner()
	fr.onCall = func(name string) { log.add("rpc:" + name) }
	s := &runnerSession{client: fr, patcher: patcher, revision: src.GetArtifact().Revision, instanceID: "loop-1"}
	return r, s, obj, fr, log, c
}

func stored(t *testing.T, c client.Client, obj *infrav1.Terraform) *infrav1.Terraform {
	t.Helper()
	out := &infrav1.Terraform{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(obj), out); err != nil {
		t.Fatalf("reading back %s: %v", obj.Name, err)
	}
	return out
}

// TestDurableCheckpointAfterPlan checks the plan outcome is written to the API
// by the plan step itself, before the step returns to its caller (reconcile or
// finalize) and so before the next runner RPC.
func TestDurableCheckpointAfterPlan(t *testing.T) {
	for _, tc := range []struct {
		name        string
		drifted     bool
		planStatus  metav1.ConditionStatus
		planReason  string
		wantPending bool
	}{
		{name: "with changes", drifted: true, planStatus: metav1.ConditionTrue, planReason: infrav1.PlannedWithChangesReason, wantPending: true},
		{name: "no changes", drifted: false, planStatus: metav1.ConditionFalse, planReason: infrav1.PlannedNoChangesReason},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			r, s, obj, fr, log, c := newLoggedPass(t, "checkpoint-plan")
			fr.replies["Plan"] = &runner.PlanReply{Message: "ok", Drifted: tc.drifted, PlanCreated: true}
			s.tfInstance = "loop-1"

			g.Expect(r.plan(context.Background(), s, obj)).To(Succeed())

			entries := log.Entries()
			save := slices.Index(entries, "rpc:SaveTFPlan")
			g.Expect(save).To(BeNumerically(">=", 0))
			// One SerialPatcher.Patch writes status and then its conditions,
			// so a checkpoint logs one or two status patches.
			g.Expect(entries[save+1:]).NotTo(BeEmpty(), "no checkpoint after SaveTFPlan: %v", entries)
			g.Expect(entries[save+1:]).To(HaveEach(HavePrefix("patch:")), "checkpoint after SaveTFPlan: %v", entries)

			got := stored(t, c, obj)
			plan := apimeta.FindStatusCondition(got.Status.Conditions, infrav1.ConditionTypePlan)
			g.Expect(plan).NotTo(BeNil())
			g.Expect(plan.Status).To(Equal(tc.planStatus))
			g.Expect(plan.Reason).To(Equal(tc.planReason))
			if tc.wantPending {
				g.Expect(got.Status.Plan.Pending).To(HavePrefix("plan-main-"))
			} else {
				g.Expect(got.Status.Plan.Pending).To(BeEmpty())
			}
			g.Expect(got.Status.Plan.Pending).To(Equal(obj.Status.Plan.Pending))
		})
	}
}

// TestDurableCheckpointAfterApply checks the apply outcome, success or
// failure, is written to the API before the next runner RPC: by the apply step
// itself, and on a whole auto-apply pass before outputs are read.
func TestDurableCheckpointAfterApply(t *testing.T) {
	t.Run("pass", func(t *testing.T) {
		g := NewWithT(t)
		r, s, obj, _, log, _ := newLoggedPass(t, "checkpoint-apply-pass")

		g.Expect(r.reconcileNormal(context.Background(), s, obj, helloWorldSource(newArtifactServer(t)))).To(Succeed())

		entries := log.Entries()
		apply := slices.Index(entries, "rpc:Apply")
		output := slices.Index(entries, "rpc:Output")
		g.Expect(apply).To(BeNumerically(">=", 0))
		g.Expect(output).To(BeNumerically(">", apply))

		// The patches between Apply and Output, matched to stored snapshots.
		patchesBefore := func(i int) int {
			n := 0
			for _, e := range entries[:i] {
				if strings.HasPrefix(e, "patch:") {
					n++
				}
			}
			return n
		}
		between := log.Patched()[patchesBefore(apply):patchesBefore(output)]
		g.Expect(between).NotTo(BeEmpty(), "no patch between Apply and Output: %v", entries)
		last := between[len(between)-1]
		g.Expect(conditions.IsTrue(last, infrav1.ConditionTypeApply)).To(BeTrue())
		g.Expect(last.Status.LastAppliedRevision).To(Equal(s.revision))
		g.Expect(last.Status.Plan.Pending).To(BeEmpty())
	})

	for _, tc := range []struct {
		name       string
		applyErr   error
		wantStatus metav1.ConditionStatus
	}{
		{name: "step succeeds", wantStatus: metav1.ConditionTrue},
		{name: "step fails", applyErr: errors.New("boom"), wantStatus: metav1.ConditionFalse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			r, s, obj, fr, log, c := newLoggedPass(t, "checkpoint-apply")
			s.tfInstance = "loop-1"
			obj.Status.Plan.Pending = "plan-main-b8e362c206"
			if tc.applyErr != nil {
				fr.errs["Apply"] = tc.applyErr
			}

			err := r.apply(context.Background(), s, obj)
			if tc.applyErr != nil {
				g.Expect(err).To(MatchError(ContainSubstring("boom")))
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}

			entries := log.Entries()
			g.Expect(entries[len(entries)-1]).To(HavePrefix("patch:"), "checkpoint after Apply, got %v", entries)

			got := stored(t, c, obj)
			applyCond := apimeta.FindStatusCondition(got.Status.Conditions, infrav1.ConditionTypeApply)
			g.Expect(applyCond).NotTo(BeNil())
			g.Expect(applyCond.Status).To(Equal(tc.wantStatus))
			g.Expect(got.Status.Plan.Pending).To(BeEmpty())
			if tc.applyErr == nil {
				g.Expect(got.Status.LastAppliedRevision).To(Equal(s.revision))
				g.Expect(got.Status.Plan.LastApplied).To(Equal("plan-main-b8e362c206"))
			} else {
				g.Expect(got.Status.LastAppliedRevision).To(BeEmpty())
			}
		})
	}
}

// TestCancelledPassPersistsNothing checks the deferred status patch runs on
// the reconcile context: a pass cancelled by shutdown writes nothing, so the
// restarted controller picks the object up from its last saved state.
func TestCancelledPassPersistsNothing(t *testing.T) {
	g := NewWithT(t)
	failOnCancelled := interceptor.Funcs{
		Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, p client.Patch, opts ...client.PatchOption) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			return c.Patch(ctx, obj, p, opts...)
		},
		SubResourcePatch: func(ctx context.Context, c client.Client, sub string, obj client.Object, p client.Patch, opts ...client.SubResourcePatchOption) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			return c.SubResource(sub).Patch(ctx, obj, p, opts...)
		},
	}
	obj := fakeTerraform("final-patch-reconcile")
	obj.Finalizers = nil
	c, _ := newInterceptedFakePatcher(t, failOnCancelled, obj)
	r := newFakeReconciler(c)
	ready := make(chan struct{})
	close(ready)
	r.CertRotator = &mtls.CertRotator{Ready: ready}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Without a finalizer, Reconcile adds one and returns; only the
	// deferred patch writes it, and it fails on the cancelled context.
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(obj)})
	g.Expect(err).To(MatchError(ContainSubstring(context.Canceled.Error())))
	g.Expect(stored(t, c, obj).Finalizers).NotTo(ContainElement(infrav1.TerraformFinalizer))
}

// TestProgressMilestonesPatched checks each progress milestone of a happy
// auto-apply pass is written to the API, in order, with the milestone rule
// the goldens use. The Reconciling and Progressing checkpoints run before the
// runner exists and are pinned by the Test_0004 goldens; the object starts in
// the state they leave it in.
func TestProgressMilestonesPatched(t *testing.T) {
	g := NewWithT(t)
	r, s, obj, _, log, _ := newLoggedPass(t, "milestones")
	conditions.MarkReconciling(obj, meta.ProgressingReason, "Fulfilling prerequisites")
	infrav1.TerraformProgressing(obj, "Reconciliation in progress")
	before := obj.DeepCopy()

	g.Expect(r.reconcileNormal(context.Background(), s, obj, helloWorldSource(newArtifactServer(t)))).To(Succeed())

	timeline := []projection{projectionOf(before)}
	for _, p := range log.Patched() {
		timeline = append(timeline, projectionOf(p))
	}
	g.Expect(milestones(timeline)).To(Equal([]string{
		"Ready/Reconciliation in progress",
		"Reconciling/Fulfilling prerequisites",
		"Ready/Initializing",
		"Ready/Terraform Planning",
		"Ready/Applying",
		"Apply/Apply started",
	}))
}

// TestInitializingPatchedBeforeNewTerraform checks the runner contract:
// "Initializing" is written to the API before the NewTerraform RPC.
func TestInitializingPatchedBeforeNewTerraform(t *testing.T) {
	g := NewWithT(t)
	r, s, obj, _, log, _ := newLoggedPass(t, "init-before-newtf")

	g.Expect(r.setup(context.Background(), s, obj, helloWorldSource(newArtifactServer(t)))).To(Succeed())

	entries := log.Entries()
	initializing := slices.Index(entries, "patch:Initializing")
	newTerraform := slices.Index(entries, "rpc:NewTerraform")
	g.Expect(initializing).To(BeNumerically(">=", 0), "no Initializing patch: %v", entries)
	g.Expect(newTerraform).To(BeNumerically(">", initializing), fmt.Sprintf("order: %v", entries))
}

// TestDriftDoesNotIncrementFailures checks a drift outcome leaves the failure
// count alone (so it can't trip RetryLimitReached or grow the backoff), while a
// real failure still counts.
func TestDriftDoesNotIncrementFailures(t *testing.T) {
	g := NewWithT(t)
	obj := fakeTerraform("drift-failures")
	c, patcher := newFakePatcher(t, obj)
	r := newFakeReconciler(c)
	src := helloWorldSource(newArtifactServer(t))

	res, err := r.result(context.Background(), patcher, obj, src, fmt.Errorf("wrapped: %w", errDriftDetected), time.Now())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(obj.GetReconciliationFailures()).To(BeZero())
	g.Expect(res.RequeueAfter).To(Equal(obj.GetRetryInterval()))
	g.Expect(obj.Status.LastSuccessfulReconcileAt).To(BeNil())

	_, err = r.result(context.Background(), patcher, obj, src, errors.New("boom"), time.Now())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(obj.GetReconciliationFailures()).To(Equal(int64(1)))
}

// TestProgressFailureLogsStep checks a failed progress patch logs which step it
// was recording, so the log tells a failure before planning from one before
// applying.
func TestProgressFailureLogsStep(t *testing.T) {
	g := NewWithT(t)
	rejectPatch := interceptor.Funcs{
		SubResourcePatch: func(context.Context, client.Client, string, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
			return errors.New("api down")
		},
	}
	obj := fakeTerraform("progress-step")
	c, patcher := newInterceptedFakePatcher(t, rejectPatch, obj)
	r := newFakeReconciler(c)
	s := &runnerSession{client: newFakeRunner(), patcher: patcher, revision: "rev"}

	var logged []string
	ctx := ctrl.LoggerInto(context.Background(), funcr.New(func(_, args string) { logged = append(logged, args) }, funcr.Options{}))

	g.Expect(r.plan(ctx, s, obj)).To(MatchError("api down"))
	g.Expect(logged).To(ContainElement(ContainSubstring("unable to update status before Terraform planning")))
}
