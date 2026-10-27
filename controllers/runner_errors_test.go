package controllers

import (
	"context"
	"errors"
	"strings"
	"testing"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func withStateLock(t *testing.T, code codes.Code, msg, id string) error {
	t.Helper()
	st, err := status.New(code, msg).WithDetails(&runner.StateLock{Id: id})
	if err != nil {
		t.Fatal(err)
	}
	return st.Err()
}

func TestStateLockFrom(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		wantOK bool
		wantID string
	}{
		{name: "aborted with detail", err: withStateLock(t, codes.Aborted, "locked", "abc"), wantOK: true, wantID: "abc"},
		{name: "aborted with detail, as the client sees it", err: newRunnerError(withStateLock(t, codes.Aborted, "locked", "abc")), wantOK: true, wantID: "abc"},
		{name: "aborted without detail", err: status.Error(codes.Aborted, "locked")},
		{name: "not locked", err: withStateLock(t, codes.FailedPrecondition, "not locked", "abc")},
		{name: "internal", err: status.Error(codes.Internal, "boom")},
		{name: "plain error", err: errors.New("boom")},
		{name: "nil", err: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			lock, ok := stateLockFrom(tt.err)

			g.Expect(ok).To(Equal(tt.wantOK))
			if tt.wantOK {
				g.Expect(lock.GetId()).To(Equal(tt.wantID))
			} else {
				g.Expect(lock).To(BeNil())
			}
		})
	}
}

func TestIsNotLocked(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "failed precondition with detail", err: withStateLock(t, codes.FailedPrecondition, "not locked", "abc"), want: true},
		{name: "failed precondition with detail, as the client sees it", err: newRunnerError(withStateLock(t, codes.FailedPrecondition, "not locked", "abc")), want: true},
		// A missing or mismatched session is also FailedPrecondition, but
		// carries no detail.
		{name: "session error", err: status.Error(codes.FailedPrecondition, "terraform session error")},
		{name: "lock held", err: withStateLock(t, codes.Aborted, "locked", "abc")},
		{name: "plain error", err: errors.New("boom")},
		{name: "nil", err: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			NewWithT(t).Expect(isNotLocked(tt.err)).To(Equal(tt.want))
		})
	}
}

func TestRunnerErrorInterceptor(t *testing.T) {
	intercept := runnerErrorInterceptor()

	t.Run("status error", func(t *testing.T) {
		g := NewWithT(t)
		invoker := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
			return status.Error(codes.NotFound, "x")
		}

		err := intercept(context.Background(), "/runner.Runner/Plan", nil, nil, nil, invoker)

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(Equal("x"))
		g.Expect(status.Code(err)).To(Equal(codes.NotFound))
		st, ok := status.FromError(err)
		g.Expect(ok).To(BeTrue())
		g.Expect(st.Message()).To(Equal("x"))
	})

	t.Run("detail survives", func(t *testing.T) {
		g := NewWithT(t)
		invoker := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
			return withStateLock(t, codes.Aborted, "locked", "abc")
		}

		err := intercept(context.Background(), "/runner.Runner/Plan", nil, nil, nil, invoker)

		g.Expect(err.Error()).To(Equal("locked"))
		lock, ok := stateLockFrom(err)
		g.Expect(ok).To(BeTrue())
		g.Expect(lock.GetId()).To(Equal("abc"))
	})

	t.Run("non-status error", func(t *testing.T) {
		g := NewWithT(t)
		invoker := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
			return errors.New("boom")
		}

		err := intercept(context.Background(), "/runner.Runner/Plan", nil, nil, nil, invoker)

		g.Expect(err.Error()).To(Equal("boom"))
		g.Expect(status.Code(err)).To(Equal(codes.Unknown))
	})

	t.Run("success", func(t *testing.T) {
		g := NewWithT(t)
		invoker := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
			return nil
		}

		g.Expect(intercept(context.Background(), "/runner.Runner/Plan", nil, nil, nil, invoker)).To(Succeed())
	})
}

// TestStateLockReachesConditions checks that every step that can find the
// state locked decodes the lock the runner reports and records its ID: the
// StateLocked condition names it and it becomes the pending lock.
func TestStateLockReachesConditions(t *testing.T) {
	const lockID = "9db590f1-b6fe-c5f2-2678-8804f089deba"

	tests := []struct {
		name string
		rpc  string
		// prepare adjusts the object so the step reaches the RPC.
		prepare func(obj *infrav1.Terraform)
		run     func(r *TerraformReconciler, s *runnerSession, obj *infrav1.Terraform) error
	}{
		{
			name: "plan",
			rpc:  "Plan",
			run: func(r *TerraformReconciler, s *runnerSession, obj *infrav1.Terraform) error {
				return r.plan(context.Background(), s, obj)
			},
		},
		{
			name: "drift detection",
			rpc:  "Plan",
			run: func(r *TerraformReconciler, s *runnerSession, obj *infrav1.Terraform) error {
				return r.detectDrift(context.Background(), s, obj)
			},
		},
		{
			name: "apply",
			rpc:  "Apply",
			prepare: func(obj *infrav1.Terraform) {
				obj.Status.Plan.Pending = "plan-main-b8e362c206"
			},
			run: func(r *TerraformReconciler, s *runnerSession, obj *infrav1.Terraform) error {
				return r.apply(context.Background(), s, obj)
			},
		},
		{
			name: "destroy",
			rpc:  "Destroy",
			prepare: func(obj *infrav1.Terraform) {
				// Destroy runs in place of apply only when the backend is
				// disabled.
				obj.Spec.BackendConfig = &infrav1.BackendConfigSpec{Disable: true}
				obj.Spec.Destroy = true
				obj.Status.Plan.Pending = "plan-main-b8e362c206"
			},
			run: func(r *TerraformReconciler, s *runnerSession, obj *infrav1.Terraform) error {
				return r.apply(context.Background(), s, obj)
			},
		},
		{
			name: "init",
			rpc:  "Init",
			run: func(r *TerraformReconciler, s *runnerSession, obj *infrav1.Terraform) error {
				return r.setup(context.Background(), s, obj, helloWorldSource(newArtifactServer(t)))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			r, s, obj, fr, _, _ := newLoggedPass(t, "state-lock-"+strings.ReplaceAll(tt.name, " ", "-"))
			s.tfInstance = "loop-1"
			if tt.prepare != nil {
				tt.prepare(obj)
			}
			// The runner reports a held lock by mapping this error to a
			// status; the fake applies the same mapping.
			fr.errs[tt.rpc] = &runner.StateLockError{ID: lockID, Path: "terraform.tfstate", Operation: "OperationTypePlan"}

			err := tt.run(r, s, obj)

			g.Expect(err).To(HaveOccurred())
			g.Expect(fr.Calls()).To(ContainElement(tt.rpc))
			locked := conditions.Get(obj, infrav1.ConditionTypeStateLocked)
			g.Expect(locked).NotTo(BeNil())
			g.Expect(locked.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(locked.Reason).To(Equal(infrav1.TFExecLockHeldReason))
			g.Expect(locked.Message).To(Equal("Terraform Locked with Lock Identifier: " + lockID))
			g.Expect(obj.Status.Lock.Pending).To(Equal(lockID))
		})
	}
}

// TestForceUnlockFindsAnotherHolder checks that when the lock being force
// unlocked is now held by someone else, the new holder's lock is recorded in
// place of the old one, and the state is not reported as unlocked.
func TestForceUnlockFindsAnotherHolder(t *testing.T) {
	const (
		oldID = "9db590f1-b6fe-c5f2-2678-8804f089deba"
		newID = "5b4e9f2a-1c3d-4e5f-8a9b-0c1d2e3f4a5b"
	)
	g := NewWithT(t)
	r, s, obj, fr, _, _ := newLoggedPass(t, "force-unlock-another-holder")
	s.tfInstance = "loop-1"
	obj.Spec.TFState = &infrav1.TFStateSpec{ForceUnlock: infrav1.ForceUnlockEnumAuto}
	obj.Status.Lock.Pending = oldID
	// The runner reports the other holder's lock as a held lock.
	fr.errs["ForceUnlock"] = &runner.StateLockError{ID: newID, Path: "terraform.tfstate", Operation: "OperationTypeApply"}

	err := r.setup(context.Background(), s, obj, helloWorldSource(newArtifactServer(t)))

	g.Expect(err).To(HaveOccurred())
	g.Expect(fr.Requests("ForceUnlock")).To(HaveLen(1))
	g.Expect(fr.Requests("ForceUnlock")[0].(*runner.ForceUnlockRequest).LockIdentifier).To(Equal(oldID))
	locked := conditions.Get(obj, infrav1.ConditionTypeStateLocked)
	g.Expect(locked).NotTo(BeNil())
	g.Expect(locked.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(locked.Reason).To(Equal(infrav1.TFExecLockHeldReason))
	g.Expect(locked.Message).To(Equal("Terraform Locked with Lock Identifier: " + newID))
	g.Expect(obj.Status.Lock.Pending).To(Equal(newID))
	ready := conditions.Get(obj, meta.ReadyCondition)
	g.Expect(ready).NotTo(BeNil())
	g.Expect(ready.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(ready.Reason).To(Equal(infrav1.TFExecLockHeldReason))
}
