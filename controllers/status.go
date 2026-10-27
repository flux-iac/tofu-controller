package controllers

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/patch"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
)

// notReady marks obj not ready at revision with reason and err's text as the
// message, and returns err, so Ready and the returned error always agree.
func notReady(obj *infrav1.Terraform, revision, reason string, err error) error {
	infrav1.TerraformNotReady(obj, revision, reason, err.Error())
	return err
}

// patchStatus patches obj's status now, logging msg if the patch fails.
func (r *TerraformReconciler) patchStatus(ctx context.Context, patcher *patch.SerialPatcher, obj *infrav1.Terraform, msg string) error {
	if err := patcher.Patch(ctx, obj, r.patchOptions...); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, msg)
		return err
	}
	return nil
}

// progress records a milestone and patches status so users see it immediately.
// The caller sets the milestone condition first; msg, which names the step, is
// logged if the patch fails.
func (r *TerraformReconciler) progress(ctx context.Context, s *runnerSession, obj *infrav1.Terraform, msg string) error {
	return r.patchStatus(ctx, s.patcher, obj, msg)
}

// checkpoint patches status after a durable outcome (plan recorded, apply finished).
func (r *TerraformReconciler) checkpoint(ctx context.Context, s *runnerSession, obj *infrav1.Terraform) error {
	return r.patchStatus(ctx, s.patcher, obj, "unable to write status checkpoint")
}

// patchFinalStatus is Reconcile's deferred status patch. It records
// the handled reconcile request, sets ObservedGeneration when Ready is True,
// patches, and records the duration. A patch error is aggregated into retErr.
// It patches on the reconcile context, so a pass cancelled by shutdown saves
// nothing and the restarted controller resumes from the last saved state.
func (r *TerraformReconciler) patchFinalStatus(ctx context.Context, patcher *patch.SerialPatcher, obj *infrav1.Terraform, start time.Time, retErr error) error {
	if v, ok := meta.ReconcileAnnotationValue(obj.GetAnnotations()); ok {
		obj.Status.SetLastHandledReconcileRequest(v)
	}

	if conditions.IsTrue(obj, meta.ReadyCondition) {
		obj.Status.ObservedGeneration = obj.Generation
	}

	if err := patcher.Patch(ctx, obj, r.patchOptions...); err != nil {
		if !obj.DeletionTimestamp.IsZero() {
			err = kerrors.FilterOut(err, func(e error) bool { return apierrors.IsNotFound(e) })
		}
		retErr = kerrors.NewAggregate([]error{retErr, err})
	}

	r.RecordDuration(ctx, obj, start)
	return retErr
}

// retryLimitReached marks obj Stalled once remediation retries are exhausted
// and stops requeueing.
func (r *TerraformReconciler) retryLimitReached(ctx context.Context, patcher *patch.SerialPatcher, obj *infrav1.Terraform) (ctrl.Result, error) {
	// `ShouldRetry` returns true when .Spec.Remediation is nil, so it is set here.
	ctrl.LoggerFrom(ctx).Info("Resource reached maximum number of retries",
		"failures", obj.GetReconciliationFailures(),
		"retries", obj.Spec.Remediation.Retries,
		"generation", obj.GetGeneration())

	infrav1.TerraformReachedLimit(obj)
	conditions.Delete(obj, meta.ReconcilingCondition)

	if err := r.patchStatus(ctx, patcher, obj, "unable to update status after the reconciliation is complete"); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// result records the pass outcome in status, patches it, and maps it to a
// requeue.
func (r *TerraformReconciler) result(ctx context.Context, patcher *patch.SerialPatcher, obj *infrav1.Terraform, src sourcev1.Source, reconcileErr error, start time.Time) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	if reconcileErr == nil {
		log.Info("Reset reconciliation failures count. Reason: successful reconciliation")
		infrav1.TerraformResetRetry(obj)
		obj.Status.LastSuccessfulReconcileAt = &metav1.Time{Time: time.Now()}
	} else if !errors.Is(reconcileErr, errDriftDetected) {
		// Drift is not a failure: it must not count toward RetryLimitReached or grow the backoff.
		obj.IncrementReconciliationFailures()
	}

	if err := r.patchStatus(ctx, patcher, obj, "unable to update status after the reconciliation is complete"); err != nil {
		return ctrl.Result{}, err
	}

	if errors.Is(reconcileErr, errDriftDetected) {
		log.Error(reconcileErr, "Drift detected",
			"after", time.Since(start).String(),
			"nextTryIn", obj.GetRetryInterval().String(),
			"revision", src.GetArtifact().Revision)

		return ctrl.Result{RequeueAfter: obj.GetRetryInterval()}, nil
	} else if reconcileErr != nil {
		// broadcast the reconciliation failure and requeue at the specified retry interval
		log.Error(reconcileErr, "Reconciliation failed",
			"after", time.Since(start).String(),
			"nextTryIn", obj.GetRetryInterval().String(),
			"revision", src.GetArtifact().Revision)
		r.Eventf(obj, nil, corev1.EventTypeWarning, infrav1.ReconciliationFailureReason, infrav1.ActionReconcile.String(), "%s", reconcileErr.Error())

		if obj.Spec.Remediation != nil {
			log.Info("Reconciliation failed, retrying",
				"failures", obj.GetReconciliationFailures(),
				"retries", obj.Spec.Remediation.Retries,
				"retryAfter", obj.GetRetryInterval(),
				"generation", obj.GetGeneration())
		} else {
			log.Info("Reconciliation failed, retrying",
				"retryAfter", obj.GetRetryInterval(),
				"generation", obj.GetGeneration())
		}

		return ctrl.Result{RequeueAfter: obj.GetRetryInterval()}, nil
	}

	log.Info("Reconciliation completed", "generation", obj.GetGeneration())

	if obj.Status.Plan.Pending != "" && !r.forceOrAutoApply(obj) {
		log.Info("Reconciliation is stopped to wait for manual operations")
		return ctrl.Result{}, nil
	}

	// next reconcile is .Spec.Interval in the future
	log.Info("requeue after interval", "interval", obj.Spec.Interval.Duration.String())
	return ctrl.Result{RequeueAfter: obj.Spec.Interval.Duration}, nil
}

// markReadyIfSettled flips an Unknown Ready condition to True and removes the
// Reconciling condition, unless Ready is still Progressing or a plan is
// waiting for approval.
func markReadyIfSettled(obj *infrav1.Terraform) {
	var (
		readyCondition      *metav1.Condition
		readyConditionIndex int
	)
	for i, condition := range obj.Status.Conditions {
		if condition.Type == meta.ReadyCondition {
			readyCondition = &condition
			readyConditionIndex = i
			break
		}
	}

	if readyCondition == nil || readyCondition.Status != metav1.ConditionUnknown {
		return
	}
	awaitingApproval := readyCondition.Reason == infrav1.PlannedWithChangesReason && strings.HasPrefix(readyCondition.Message, "Plan generated")
	if awaitingApproval || readyCondition.Reason == meta.ProgressingReason {
		return
	}
	obj.Status.Conditions[readyConditionIndex].Status = metav1.ConditionTrue
	conditions.Delete(obj, meta.ReconcilingCondition) // do at the end because it shifts indices
}
