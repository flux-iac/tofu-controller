package controllers

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/mtls"
	"github.com/flux-iac/tofu-controller/runner"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// noDestroyRunner is the fake runner with Plan, Apply and Destroy failing the
// test: a deletion that must not destroy never reaches them.
type noDestroyRunner struct {
	*fakeRunner
	t *testing.T
}

func (f noDestroyRunner) Plan(ctx context.Context, in *runner.PlanRequest, opts ...grpc.CallOption) (*runner.PlanReply, error) {
	f.t.Errorf("unexpected Plan during a deletion without destroy")
	return f.fakeRunner.Plan(ctx, in, opts...)
}

func (f noDestroyRunner) Apply(ctx context.Context, in *runner.ApplyRequest, opts ...grpc.CallOption) (*runner.ApplyReply, error) {
	f.t.Errorf("unexpected Apply during a deletion without destroy")
	return f.fakeRunner.Apply(ctx, in, opts...)
}

func (f noDestroyRunner) Destroy(ctx context.Context, in *runner.DestroyRequest, opts ...grpc.CallOption) (*runner.DestroyReply, error) {
	f.t.Errorf("unexpected Destroy during a deletion without destroy")
	return f.fakeRunner.Destroy(ctx, in, opts...)
}

// deleteFixture is a reconciler on a fake API server holding obj, already
// marked for deletion, with the runner replaced by a noDestroyRunner.
type deleteFixture struct {
	r *TerraformReconciler
	c client.Client
	// fr records the runner RPCs.
	fr *fakeRunner
	// revisions holds the revision of each runner session opened.
	revisions []string
}

func newDeleteFixture(t *testing.T, obj *infrav1.Terraform, others ...client.Object) *deleteFixture {
	t.Helper()
	return newInterceptedDeleteFixture(t, interceptor.Funcs{}, obj, others...)
}

// newInterceptedDeleteFixture is newDeleteFixture with interceptor funcs on the
// fake API client.
func newInterceptedDeleteFixture(t *testing.T, funcs interceptor.Funcs, obj *infrav1.Terraform, others ...client.Object) *deleteFixture {
	t.Helper()
	t.Setenv("INSECURE_LOCAL_RUNNER", "1")
	t.Setenv("DISABLE_TF_K8S_BACKEND", "1")

	now := metav1.Now()
	obj.DeletionTimestamp = &now
	c, _ := newInterceptedFakePatcher(t, funcs, obj, others...)

	f := &deleteFixture{c: c, fr: newFakeRunner()}
	f.r = newFakeReconciler(c)
	ready := make(chan struct{})
	close(ready)
	f.r.CertRotator = &mtls.CertRotator{Ready: ready}
	f.r.lookupOrCreateRunner = func(_ context.Context, _ *infrav1.Terraform, revision string) (runner.RunnerClient, func() error, error) {
		f.revisions = append(f.revisions, revision)
		return noDestroyRunner{fakeRunner: f.fr, t: t}, func() error { return nil }, nil
	}
	return f
}

func (f *deleteFixture) reconcile(t *testing.T, obj *infrav1.Terraform) (ctrl.Result, error) {
	t.Helper()
	return f.r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(obj)})
}

func (f *deleteFixture) gone(t *testing.T, obj *infrav1.Terraform) bool {
	t.Helper()
	err := f.c.Get(context.Background(), client.ObjectKeyFromObject(obj), &infrav1.Terraform{})
	return apierrors.IsNotFound(err)
}

// TestReconcileDeleteSuspendedSkipsDestroy checks that a suspended object is
// deleted, without destroy even when destroyResourcesOnDeletion is set, and
// its secrets are still finalized.
func TestReconcileDeleteSuspendedSkipsDestroy(t *testing.T) {
	g := NewWithT(t)
	obj := fakeTerraform("delete-suspended")
	obj.Spec.Suspend = true
	obj.Spec.DestroyResourcesOnDeletion = true
	// No source object: a suspended deletion does not need it.
	f := newDeleteFixture(t, obj)

	res, err := f.reconcile(t, obj)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res).To(Equal(ctrl.Result{}))

	g.Expect(f.fr.Calls()).To(Equal([]string{"FinalizeSecrets"}))
	g.Expect(f.gone(t, obj)).To(BeTrue(), "the suspended object should be deleted")

	// The skipped destroy is reported, since the object's status goes with it.
	recorded := drainEvents(f.r.EventRecorder.(*events.FakeRecorder))
	g.Expect(recorded).To(HaveLen(1))
	g.Expect(recorded[0]).To(ContainSubstring(corev1.EventTypeNormal))
	g.Expect(recorded[0]).To(ContainSubstring(infrav1.DestroySkippedReason))
}

// TestReconcileDeleteWithoutSourceSkipsDestroyWhenNotRequested checks that
// without destroyResourcesOnDeletion a missing source does not block
// deletion, and the runner session carries no source revision.
func TestReconcileDeleteWithoutSourceSkipsDestroyWhenNotRequested(t *testing.T) {
	g := NewWithT(t)
	obj := fakeTerraform("delete-no-source")
	obj.Status.LastAttemptedRevision = "main@sha1:b8e362c206e3d0cbb7ed22ced771a0056455a2fb"
	f := newDeleteFixture(t, obj) // the GitRepository does not exist

	res, err := f.reconcile(t, obj)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res).To(Equal(ctrl.Result{}))

	g.Expect(f.fr.Calls()).To(Equal([]string{"FinalizeSecrets"}))
	g.Expect(f.revisions).To(Equal([]string{""}))
	g.Expect(f.gone(t, obj)).To(BeTrue(), "the object should be deleted without its source")
	// Destroy was not requested, so there is no skipped destroy to report.
	g.Expect(drainEvents(f.r.EventRecorder.(*events.FakeRecorder))).To(BeEmpty())
}

// TestReconcileDeleteReturnsAfterFinalize checks that once the finalizer is
// removed the pass ends. Nothing past finalize runs: no retry-limit check,
// no pipeline, no result mapping.
func TestReconcileDeleteReturnsAfterFinalize(t *testing.T) {
	g := NewWithT(t)
	src := helloWorldSource(newArtifactServer(t))
	obj := fakeTerraform("delete-returns")
	// Retries exhausted: falling through would reach the retry-limit check and
	// mark it Stalled.
	obj.Spec.Remediation = &infrav1.Remediation{Retries: 1}
	obj.Status.ReconciliationFailures = 1
	obj.Status.LastAttemptedRevision = src.GetArtifact().Revision
	// Generation already observed, so the retry count is not reset on the way in.
	obj.Status.ObservedGeneration = obj.Generation
	f := newDeleteFixture(t, obj, src)

	var recorded *infrav1.Terraform
	f.r.observeReconcile = func(_ ctrl.Request, o *infrav1.Terraform, _ ctrl.Result, _ error, _ bool) {
		recorded = o.DeepCopy()
	}

	res, err := f.reconcile(t, obj)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res).To(Equal(ctrl.Result{}))

	g.Expect(f.fr.Calls()).To(Equal([]string{"FinalizeSecrets"}))
	g.Expect(f.gone(t, obj)).To(BeTrue())
	g.Expect(recorded).NotTo(BeNil())
	g.Expect(conditions.Has(recorded, meta.StalledCondition)).To(BeFalse(), "the retry-limit check ran after finalize")
}

// TestReconcileDeleteWithoutOwnFinalizerDoesNothing checks a deletion whose
// object no longer carries (or never got) the controller's finalizer opens no
// runner and adds no finalizer: there is nothing of ours to clean up.
func TestReconcileDeleteWithoutOwnFinalizerDoesNothing(t *testing.T) {
	g := NewWithT(t)
	obj := fakeTerraform("delete-no-finalizer")
	// Another controller's finalizer keeps the deleting object around.
	obj.Finalizers = []string{"example.com/other"}
	f := newDeleteFixture(t, obj)

	res, err := f.reconcile(t, obj)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res).To(Equal(ctrl.Result{}))

	g.Expect(f.revisions).To(BeEmpty(), "no runner session should be opened")
	g.Expect(stored(t, f.c, obj).Finalizers).To(Equal([]string{"example.com/other"}))
}

// TestRetryLimitCheckedBeforeRunner checks that when retries are exhausted the
// pass stops at the retry-limit check before the runner pod is created.
func TestRetryLimitCheckedBeforeRunner(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("INSECURE_LOCAL_RUNNER", "") // take the runner pod path

	src := helloWorldSource(newArtifactServer(t))
	obj := fakeTerraform("retry-before-runner")
	obj.Spec.Remediation = &infrav1.Remediation{Retries: 1}
	obj.Spec.RunnerTerminationGracePeriodSeconds = new(int64)
	obj.Status.ReconciliationFailures = 1
	// Same revision and generation as last time, so the retry count is not reset.
	obj.Status.LastAttemptedRevision = src.GetArtifact().Revision
	obj.Status.ObservedGeneration = 1

	var mu sync.Mutex
	podCreates := 0
	funcs := interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, o client.Object, opts ...client.CreateOption) error {
			if _, ok := o.(*corev1.Pod); ok {
				mu.Lock()
				podCreates++
				mu.Unlock()
				return errors.New("pod create refused by the test")
			}
			return c.Create(ctx, o, opts...)
		},
	}
	c, _ := newInterceptedFakePatcher(t, funcs, obj, src)
	r := newFakeReconciler(c)
	r.RunnerCreationTimeout = time.Second

	// The cert rotator answers TLS secret requests until the test ends.
	ready := make(chan struct{})
	close(ready)
	tlsTriggers := make(chan mtls.Trigger)
	done := make(chan struct{})
	t.Cleanup(func() { close(done) })
	go func() {
		for {
			select {
			case trig := <-tlsTriggers:
				trig.Ready <- &mtls.TriggerResult{Secret: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "terraform-runner.tls-1", Namespace: trig.Namespace},
				}}
			case <-done:
				return
			}
		}
	}()
	r.CertRotator = &mtls.CertRotator{Ready: ready, TriggerNamespaceTLSGeneration: tlsTriggers}

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(obj)})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res).To(Equal(ctrl.Result{}))

	mu.Lock()
	g.Expect(podCreates).To(BeZero(), "runner pod created before the retry limit was checked")
	mu.Unlock()
	got := stored(t, c, obj)
	g.Expect(conditions.IsTrue(got, meta.StalledCondition)).To(BeTrue())
	g.Expect(conditions.GetReason(got, meta.StalledCondition)).To(Equal(infrav1.RetryLimitReachedReason))
}

// TestRunnerPodInstanceFallback checks the runner pod instance label: the
// source revision when there is one, otherwise the last attempted revision,
// then the last applied one, then a fixed value.
func TestRunnerPodInstanceFallback(t *testing.T) {
	const (
		attempted = "main@sha1:1111111122222222333333334444444455555555"
		applied   = "main@sha1:aaaaaaaabbbbbbbbccccccccddddddddeeeeeeee"
		source    = "main@sha1:b8e362c206e3d0cbb7ed22ced771a0056455a2fb"
	)
	tests := []struct {
		name      string
		revision  string
		attempted string
		applied   string
		want      string
		wantErr   bool
	}{
		{name: "source revision", revision: source, attempted: attempted, applied: applied, want: "tf-runner-b8e362c2"},
		{name: "unparsable source revision still fails", revision: "master/b8e362c2", wantErr: true},
		{name: "no source: last attempted", attempted: attempted, applied: applied, want: "tf-runner-11111111"},
		{name: "no source: last applied", applied: applied, want: "tf-runner-aaaaaaaa"},
		{name: "no source: unparsable attempted falls back to applied", attempted: "master/1234", applied: applied, want: "tf-runner-aaaaaaaa"},
		{name: "no source, no revisions", want: "tf-runner-finalize"},
		{name: "no source, unparsable revisions", attempted: "master/1234", applied: "main@sha1:abc", want: "tf-runner-finalize"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			obj := fakeTerraform("instance")
			obj.Status.LastAttemptedRevision = tt.attempted
			obj.Status.LastAppliedRevision = tt.applied

			got, err := runnerPodInstance(obj, tt.revision)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

// TestFinalizeSecretsUnexpectedErrorEmitsWarning checks that a FinalizeSecrets
// error other than Internal, Unavailable and NotFound, gRPC or not, is reported as a
// Warning event and deletion proceeds.
func TestFinalizeSecretsUnexpectedErrorEmitsWarning(t *testing.T) {
	for name, rpcErr := range map[string]error{
		"PermissionDenied": status.Error(codes.PermissionDenied, "no access"),
		"non-gRPC":         errors.New("boom"),
	} {
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)
			obj := fakeTerraform("finalize-secrets-" + strings.ToLower(name))
			f := newDeleteFixture(t, obj)
			f.fr.errs["FinalizeSecrets"] = rpcErr

			res, err := f.reconcile(t, obj)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(res).To(Equal(ctrl.Result{}))
			g.Expect(f.gone(t, obj)).To(BeTrue(), "deletion should proceed")

			recorded := drainEvents(f.r.EventRecorder.(*events.FakeRecorder))
			g.Expect(recorded).To(HaveLen(1))
			g.Expect(recorded[0]).To(ContainSubstring(corev1.EventTypeWarning))
			g.Expect(recorded[0]).To(ContainSubstring(infrav1.FinalizeSecretsFailedReason))
			g.Expect(recorded[0]).To(ContainSubstring(rpcErr.Error()))
		})
	}
}

// TestFinalizeSecretsNotFoundIsSilent: NotFound means there was nothing to
// delete; no event, deletion proceeds.
func TestFinalizeSecretsNotFoundIsSilent(t *testing.T) {
	g := NewWithT(t)
	obj := fakeTerraform("finalize-secrets-notfound")
	f := newDeleteFixture(t, obj)
	f.fr.errs["FinalizeSecrets"] = status.Error(codes.NotFound, "no secrets")

	_, err := f.reconcile(t, obj)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(f.gone(t, obj)).To(BeTrue())
	g.Expect(drainEvents(f.r.EventRecorder.(*events.FakeRecorder))).To(BeEmpty())
}

// TestFinalizeSecretsInternalBlocksDeletion: Internal is transient and still
// returns the error, leaving the object in place.
func TestFinalizeSecretsInternalBlocksDeletion(t *testing.T) {
	g := NewWithT(t)
	obj := fakeTerraform("finalize-secrets-internal")
	f := newDeleteFixture(t, obj)
	f.fr.errs["FinalizeSecrets"] = status.Error(codes.Internal, "transient")

	_, err := f.reconcile(t, obj)
	g.Expect(err).To(HaveOccurred())
	g.Expect(f.gone(t, obj)).To(BeFalse())
	g.Expect(drainEvents(f.r.EventRecorder.(*events.FakeRecorder))).To(BeEmpty())
}

// TestFinalizeSecretsUnavailableBlocksDeletion: Unavailable means the runner
// could not be reached; the error is returned and the object stays, so its
// plan secrets are not left behind.
func TestFinalizeSecretsUnavailableBlocksDeletion(t *testing.T) {
	g := NewWithT(t)
	obj := fakeTerraform("finalize-secrets-unavailable")
	f := newDeleteFixture(t, obj)
	f.fr.errs["FinalizeSecrets"] = status.Error(codes.Unavailable, "runner unreachable")

	_, err := f.reconcile(t, obj)
	g.Expect(status.Code(err)).To(Equal(codes.Unavailable))
	g.Expect(f.gone(t, obj)).To(BeFalse())
	g.Expect(stored(t, f.c, obj).Finalizers).To(ContainElement(infrav1.TerraformFinalizer))
	g.Expect(drainEvents(f.r.EventRecorder.(*events.FakeRecorder))).To(BeEmpty())
}

// TestFinalizeTreatsMissingDependencyAsReleased checks that a dependsOn target
// that no longer exists has nothing to release, so deletion completes.
func TestFinalizeTreatsMissingDependencyAsReleased(t *testing.T) {
	g := NewWithT(t)
	obj := fakeTerraform("finalize-missing-dependency")
	obj.Spec.DependsOn = []meta.NamespacedObjectReference{{Name: "never-created"}}
	f := newDeleteFixture(t, obj)

	res, err := f.reconcile(t, obj)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res).To(Equal(ctrl.Result{}))
	g.Expect(f.gone(t, obj)).To(BeTrue(), "deletion should complete")
}

// TestFinalizeReturnsOtherDependencyGetErrors: only NotFound is treated as
// released; any other error looking up a dependsOn target is returned and the
// object stays.
func TestFinalizeReturnsOtherDependencyGetErrors(t *testing.T) {
	g := NewWithT(t)
	obj := fakeTerraform("finalize-dependency-get-error")
	dep := fakeTerraform("flaky-dependency")
	obj.Spec.DependsOn = []meta.NamespacedObjectReference{{Name: dep.Name}}
	boom := errors.New("apiserver unavailable")
	hit := false
	f := newInterceptedDeleteFixture(t, interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, o client.Object, opts ...client.GetOption) error {
			if key.Name == dep.Name {
				hit = true
				return boom
			}
			return c.Get(ctx, key, o, opts...)
		},
	}, obj, dep)

	_, err := f.reconcile(t, obj)
	// The deferred status patch can add its own conflict to the returned error,
	// so assert the lookup was reached and the pass failed, not the exact error.
	g.Expect(hit).To(BeTrue(), "dependency lookup not reached")
	g.Expect(err).To(HaveOccurred())
	g.Expect(f.gone(t, obj)).To(BeFalse())
}

// TestReconcileDeleteClearsStaleRunnerFailure checks that once the runner
// session opens, a deletion no longer reports the previous pass's runner
// failure, so a later deletion failure is not blamed on the runner.
func TestReconcileDeleteClearsStaleRunnerFailure(t *testing.T) {
	g := NewWithT(t)
	obj := fakeTerraform("delete-after-runner-failure")
	conditions.MarkFalse(obj, meta.ReadyCondition, infrav1.RunnerFailedReason, "%s", errors.New("old boom"))
	conditions.MarkReconciling(obj, meta.ProgressingReason, "Fulfilling prerequisites")
	f := newDeleteFixture(t, obj)
	f.fr.errs["FinalizeSecrets"] = status.Error(codes.Internal, "secrets api down")

	_, err := f.reconcile(t, obj)
	g.Expect(err).To(HaveOccurred())

	ready := conditions.Get(stored(t, f.c, obj), meta.ReadyCondition)
	g.Expect(ready).NotTo(BeNil())
	g.Expect(ready.Reason).NotTo(Equal(infrav1.RunnerFailedReason))
	g.Expect(ready.Message).To(Equal("Deletion in progress"))
}
