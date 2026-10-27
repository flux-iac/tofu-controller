package controllers

import (
	"context"

	"github.com/flux-iac/tofu-controller/runner"
	"github.com/flux-iac/tofu-controller/utils"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/logger"
	"github.com/fluxcd/pkg/runtime/patch"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
)

// runnerSession is everything one reconcile pass needs to talk to its runner.
type runnerSession struct {
	client     runner.RunnerClient
	closeConn  func() error
	patcher    *patch.SerialPatcher
	tfInstance string // set by setup
	tmpDir     string // set by setup; removed by cleanupDir
	revision   string // artifact revision the pass works on ("" when deleting without destroy)

	// instanceID is the reconciliation loop ID, sent to the runner as
	// NewTerraform's InstanceID; NewTerraform replies with it as tfInstance.
	instanceID string

	// cleanupRunner, when set, removes the runner pod once the connection is
	// closed.
	cleanupRunner func(ctx context.Context)
}

// openSession provisions the runner for obj and connects to it.
// reconcileID is the reconciliation loop ID, which the session sends to the
// runner as its instance ID. On error the connection, if any, is already
// closed. Close the returned session to release the runner.
func (r *TerraformReconciler) openSession(ctx context.Context, patcher *patch.SerialPatcher, obj *infrav1.Terraform, revision, reconcileID string) (*runnerSession, error) {
	log := ctrl.LoggerFrom(ctx)

	lookupOrCreate := r.LookupOrCreateRunner
	if r.lookupOrCreateRunner != nil {
		lookupOrCreate = r.lookupOrCreateRunner
	}
	runnerClient, closeConn, err := lookupOrCreate(ctx, obj, revision)
	if err != nil {
		log.Error(err, "unable to lookup or create runner")
		if closeConn != nil {
			if err := closeConn(); err != nil {
				log.Error(err, "unable to close connection")
			}
		}
		return nil, err
	}
	log.Info("runner is running")

	// The runner is up, so a runner failure from the last pass no longer
	// applies. A deletion without destroy never reaches setup's
	// "Initializing", so replace it here.
	if runnerFailed(obj) {
		infrav1.TerraformProgressing(obj, progressingMessage(obj))
	}

	return &runnerSession{
		client:        runnerClient,
		closeConn:     closeConn,
		patcher:       patcher,
		revision:      revision,
		instanceID:    reconcileID,
		cleanupRunner: func(ctx context.Context) { r.cleanupRunnerPod(ctx, obj) },
	}, nil
}

// runnerError maps a failure to open the runner session to a result: a quota
// error requeues with a jittered delay when quota retry is enabled; any other
// error is returned. Either way Ready is set False (reason
// RunnerQuotaExhausted on the quota retry path, RunnerFailed otherwise, message
// the error text). Reconciling stays, because the controller keeps retrying. It
// is used by both the normal and the deletion path.
//
// ObservedGeneration is deliberately left alone, as for source errors: the
// generation was not acted on, and recording it would make the next pass skip
// a spec change until spec.interval elapses.
func (r *TerraformReconciler) runnerError(ctx context.Context, obj *infrav1.Terraform, err error) (ctrl.Result, error) {
	if r.QuotaRetryEnabled && utils.IsQuotaError(err) {
		jitteredDelay := wait.Jitter(r.QuotaRetryDelay, r.QuotaRetryJitterFactor)
		ctrl.LoggerFrom(ctx).V(logger.TraceLevel).Info("runner quota exhausted, retrying",
			"retryAfter", jitteredDelay,
			"baseDelay", r.QuotaRetryDelay,
			"jitterFactor", r.QuotaRetryJitterFactor)
		markRunnerFailed(obj, infrav1.RunnerQuotaExhaustedReason, err)
		r.Eventf(obj, nil, corev1.EventTypeWarning, infrav1.RunnerQuotaExhaustedReason, infrav1.ActionProvisionRunner.String(),
			"runner pod creation blocked by resource quota, retrying")
		return ctrl.Result{RequeueAfter: jitteredDelay}, nil
	}

	markRunnerFailed(obj, infrav1.RunnerFailedReason, err)
	return ctrl.Result{}, err
}

// markRunnerFailed sets Ready False with reason and the error text.
func markRunnerFailed(obj *infrav1.Terraform, reason string, err error) {
	conditions.MarkFalse(obj, meta.ReadyCondition, reason, "%s", err)
}

// runnerFailed reports whether the last pass failed to open the runner session.
func runnerFailed(obj *infrav1.Terraform) bool {
	return conditions.HasAnyReason(obj, meta.ReadyCondition, infrav1.RunnerFailedReason, infrav1.RunnerQuotaExhaustedReason)
}

// Close closes the gRPC connection, then cleans up the runner pod. It logs
// errors and never returns them.
func (s *runnerSession) Close(ctx context.Context) {
	log := ctrl.LoggerFrom(ctx)

	if s.closeConn != nil {
		if err := s.closeConn(); err != nil {
			log.Error(err, "unable to close connection")
		}
	}

	if s.cleanupRunner != nil {
		s.cleanupRunner(ctx)
	}
}

// cleanupDir asks the runner to remove tmpDir. Callers defer it before setup,
// which sets tmpDir. It sends nothing while tmpDir is empty: there is no
// directory to name, and a runner that never came up would block the call
// forever (see runnerUnreachableInterceptor).
func (s *runnerSession) cleanupDir(ctx context.Context) {
	if s.tmpDir == "" {
		return
	}

	log := ctrl.LoggerFrom(ctx)

	cleanupDirReply, err := s.client.CleanupDir(ctx, &runner.CleanupDirRequest{TmpDir: s.tmpDir})
	if err != nil {
		log.Error(err, "clean up error")
	}

	if cleanupDirReply != nil {
		log.Info("clean up dir", "message", cleanupDirReply.Message)
	}
}
