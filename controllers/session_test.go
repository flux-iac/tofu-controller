package controllers

import (
	"context"
	"errors"
	"testing"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/mtls"
	"github.com/flux-iac/tofu-controller/runner"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// TestSetupSendsSnapshotAfterInitializing checks the runner contract: the
// object bytes the runner receives in NewTerraform are taken after the
// "Initializing" progress checkpoint.
func TestSetupSendsSnapshotAfterInitializing(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("DISABLE_TF_K8S_BACKEND", "1")

	obj := fakeTerraform("setup-snapshot")
	c, patcher := newFakePatcher(t, obj)
	r := newFakeReconciler(c)
	src := helloWorldSource(newArtifactServer(t))
	fr := newFakeRunner()
	s := &runnerSession{client: fr, patcher: patcher, revision: src.GetArtifact().Revision, instanceID: "loop-1"}

	g.Expect(r.setup(context.Background(), s, obj, src)).To(Succeed())

	g.Expect(fr.Calls()).To(Equal([]string{
		"UploadAndExtract", "WriteBackendConfig", "LookPath", "NewTerraform",
		"SetEnv", "GenerateVarsForTF", "GenerateTemplate", "Init", "SelectWorkspace",
	}))
	g.Expect(s.tfInstance).To(Equal("loop-1"))
	g.Expect(s.tmpDir).To(Equal("/fake/tmp"))

	reqs := fr.Requests("NewTerraform")
	g.Expect(reqs).To(HaveLen(1))
	sent := &infrav1.Terraform{}
	g.Expect(sent.FromBytes(reqs[0].(*runner.NewTerraformRequest).Terraform, r.Scheme)).To(Succeed())
	ready := apimeta.FindStatusCondition(sent.Status.Conditions, meta.ReadyCondition)
	g.Expect(ready).NotTo(BeNil())
	g.Expect(ready.Status).To(Equal(metav1.ConditionUnknown))
	g.Expect(ready.Reason).To(Equal(meta.ProgressingReason))
	g.Expect(ready.Message).To(Equal("Initializing"))

	// The checkpoint was patched, not only set in memory.
	stored := &infrav1.Terraform{}
	g.Expect(c.Get(context.Background(), client.ObjectKeyFromObject(obj), stored)).To(Succeed())
	g.Expect(apimeta.FindStatusCondition(stored.Status.Conditions, meta.ReadyCondition).Message).To(Equal("Initializing"))
}

// TestWorkingDirRemovedWhenSetupFails checks a pass whose setup fails after
// extracting the artifact still removes the working directory, and that Close
// leaves the directory to cleanupDir.
func TestWorkingDirRemovedWhenSetupFails(t *testing.T) {
	g := NewWithT(t)
	r, s, obj, fr, _, _ := newLoggedPass(t, "setup-fails-cleanup")
	fr.errs["LookPath"] = errors.New("no binaries")

	err := r.reconcileNormal(context.Background(), s, obj, helloWorldSource(newArtifactServer(t)))
	g.Expect(err).To(MatchError(ContainSubstring("no binaries")))
	g.Expect(fr.Requests("CleanupDir")).To(HaveLen(1))
	g.Expect(fr.Requests("CleanupDir")[0].(*runner.CleanupDirRequest).TmpDir).To(Equal("/fake/tmp"))

	closed := false
	s.closeConn = func() error { closed = true; return nil }
	s.Close(context.Background())
	g.Expect(closed).To(BeTrue())
	g.Expect(fr.Requests("CleanupDir")).To(HaveLen(1))
}

// TestRunnerErrorSetsReadyFalse checks that a runner error sets Ready False and
// keeps Reconciling. The reason is RunnerQuotaExhausted on the quota retry
// path and RunnerFailed otherwise; the message is the error text.
func TestRunnerErrorSetsReadyFalse(t *testing.T) {
	quotaErr := apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "x-tf-runner", errors.New("exceeded quota: q"))

	tests := []struct {
		name        string
		quotaRetry  bool
		err         error
		wantReason  string
		wantRequeue bool
	}{
		{"quota with retry", true, quotaErr, infrav1.RunnerQuotaExhaustedReason, true},
		{"quota without retry", false, quotaErr, infrav1.RunnerFailedReason, false},
		{"other error", true, errors.New("boom"), infrav1.RunnerFailedReason, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			obj := fakeTerraform("runner-error")
			conditions.MarkTrue(obj, meta.ReconcilingCondition, meta.ProgressingReason, "Reconciliation in progress")
			c, _ := newFakePatcher(t, obj)
			r := newFakeReconciler(c)
			r.QuotaRetryEnabled = tt.quotaRetry
			r.QuotaRetryDelay = time.Hour

			res, err := r.runnerError(context.Background(), obj, tt.err)

			if tt.wantRequeue {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(res.RequeueAfter).NotTo(BeZero())
			} else {
				g.Expect(err).To(Equal(tt.err))
				g.Expect(res).To(Equal(ctrl.Result{}))
			}
			ready := apimeta.FindStatusCondition(obj.Status.Conditions, meta.ReadyCondition)
			g.Expect(ready).NotTo(BeNil())
			g.Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			g.Expect(ready.Reason).To(Equal(tt.wantReason))
			g.Expect(ready.Message).To(Equal(tt.err.Error()))
			g.Expect(apimeta.FindStatusCondition(obj.Status.Conditions, meta.ReconcilingCondition)).NotTo(BeNil())
		})
	}
}

// TestRunnerErrorDoesNotSuppressGenerationChange checks that a runner failure
// leaves ObservedGeneration alone, so the next pass still sees the spec change
// and retries promptly instead of waiting for spec.interval.
func TestRunnerErrorDoesNotSuppressGenerationChange(t *testing.T) {
	quotaErr := apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "x-tf-runner", errors.New("exceeded quota: q"))

	tests := []struct {
		name string
		err  error
	}{
		{"quota error", quotaErr},
		{"generic error", errors.New("boom")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			const revision = "main@sha1:abc"
			now := metav1.Now()
			obj := fakeTerraform("runner-error-generation")
			obj.Generation = 2
			obj.Status.ObservedGeneration = 1
			obj.Status.LastPlanAt = &now
			obj.Status.LastSuccessfulReconcileAt = &now
			obj.Status.ReconciliationFailures = 0
			obj.Status.LastAttemptedRevision = revision
			src := &sourcev1.GitRepository{Status: sourcev1.GitRepositoryStatus{
				Artifact: &meta.Artifact{Revision: revision},
			}}
			c, _ := newFakePatcher(t, obj)
			r := newFakeReconciler(c)
			r.QuotaRetryEnabled = true
			r.QuotaRetryDelay = time.Hour

			_, _ = r.runnerError(context.Background(), obj, tt.err)

			g.Expect(obj.Status.ObservedGeneration).To(Equal(int64(1)))
			ok, reason, _ := r.shouldReconcile(obj, src)
			g.Expect(ok).To(BeTrue())
			g.Expect(reason).To(ContainSubstring("generation has changed"))
		})
	}
}

// TestRepeatedRunnerFailureWritesNoStatus checks that a pass hitting the same
// runner failure as the last one writes no status: the prelude leaves the
// failure in place, so retries do not flap Ready and Reconciling.
func TestRepeatedRunnerFailureWritesNoStatus(t *testing.T) {
	g := NewWithT(t)
	var writes []string
	countWrites := interceptor.Funcs{
		Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, p client.Patch, opts ...client.PatchOption) error {
			if data, err := p.Data(obj); err == nil && string(data) != "{}" {
				writes = append(writes, string(data))
			}
			return c.Patch(ctx, obj, p, opts...)
		},
		SubResourcePatch: func(ctx context.Context, c client.Client, sub string, obj client.Object, p client.Patch, opts ...client.SubResourcePatchOption) error {
			if data, err := p.Data(obj); err == nil && string(data) != "{}" {
				writes = append(writes, string(data))
			}
			return c.SubResource(sub).Patch(ctx, obj, p, opts...)
		},
	}
	src := helloWorldSource(newArtifactServer(t))
	obj := fakeTerraform("runner-repeat")
	c, _ := newInterceptedFakePatcher(t, countWrites, obj, src)
	r := newFakeReconciler(c)
	ready := make(chan struct{})
	close(ready)
	r.CertRotator = &mtls.CertRotator{Ready: ready}
	r.QuotaRetryEnabled = true
	r.QuotaRetryDelay = time.Hour
	quotaErr := apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "x-tf-runner", errors.New("exceeded quota: q"))
	r.lookupOrCreateRunner = func(context.Context, *infrav1.Terraform, string) (runner.RunnerClient, func() error, error) {
		return nil, nil, quotaErr
	}
	req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(obj)}

	_, err := r.Reconcile(context.Background(), req)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(writes).NotTo(BeEmpty())
	ready1 := apimeta.FindStatusCondition(stored(t, c, obj).Status.Conditions, meta.ReadyCondition)
	g.Expect(ready1.Reason).To(Equal(infrav1.RunnerQuotaExhaustedReason))

	writes = nil
	_, err = r.Reconcile(context.Background(), req)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(writes).To(BeEmpty())
}
