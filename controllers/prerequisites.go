package controllers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/acl"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/patch"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/hashicorp/go-retryablehttp"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// resolveArtifact fetches obj's source and checks that it has an artifact.
// When either fails, ok is false and res and err are what the pass returns,
// with status already set. When the source resolves, a Ready failure left by
// an earlier source problem is cleared.
func (r *TerraformReconciler) resolveArtifact(ctx context.Context, obj *infrav1.Terraform) (src sourcev1.Source, ok bool, res ctrl.Result, err error) {
	src, err = r.resolveSource(ctx, obj)
	if err != nil {
		res, err = r.sourceError(obj, err)
		return nil, false, res, err
	}
	markInProgressIf(obj, infrav1.AccessDeniedReason, infrav1.ArtifactFailedReason)

	if src.GetArtifact() == nil {
		res, err = r.artifactMissing(ctx, obj)
		return nil, false, res, err
	}
	return src, true, ctrl.Result{}, nil
}

func (r *TerraformReconciler) resolveSource(ctx context.Context, terraform *infrav1.Terraform) (sourcev1.Source, error) {
	var sourceObj sourcev1.Source

	name, namespace := terraform.Spec.SourceRef.Name, terraform.Spec.SourceRef.Namespace
	if namespace == "" {
		namespace = terraform.GetNamespace()
	}

	sourceReference := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}

	if r.NoCrossNamespaceRefs && sourceReference.Namespace != terraform.GetNamespace() {
		return nil, acl.AccessDeniedError(
			fmt.Sprintf("cannot access %s/%s, cross-namespace references have been disabled", terraform.Spec.SourceRef.Kind, sourceReference),
		)
	}

	switch terraform.Spec.SourceRef.Kind {
	case sourcev1.GitRepositoryKind:
		var repository sourcev1.GitRepository
		err := r.Get(ctx, sourceReference, &repository)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return sourceObj, err
			}
			return sourceObj, fmt.Errorf("unable to get source '%s': %w", sourceReference, err)
		}
		sourceObj = &repository
	case sourcev1.BucketKind:
		var bucket sourcev1.Bucket
		err := r.Get(ctx, sourceReference, &bucket)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return sourceObj, err
			}
			return sourceObj, fmt.Errorf("unable to get source '%s': %w", sourceReference, err)
		}
		sourceObj = &bucket
	case sourcev1.OCIRepositoryKind:
		var repository sourcev1.OCIRepository
		err := r.Get(ctx, sourceReference, &repository)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return sourceObj, err
			}
			return sourceObj, fmt.Errorf("unable to get source '%s': %w", sourceReference, err)
		}
		sourceObj = &repository
	default:
		return sourceObj, fmt.Errorf("source `%s` kind '%s' not supported",
			terraform.Spec.SourceRef.Name, terraform.Spec.SourceRef.Kind)
	}

	return sourceObj, nil
}

func (r *TerraformReconciler) downloadAsBytes(artifact *meta.Artifact) (*bytes.Buffer, error) {
	artifactURL := artifact.URL
	if hostname := os.Getenv("SOURCE_CONTROLLER_LOCALHOST"); hostname != "" {
		u, err := url.Parse(artifactURL)
		if err != nil {
			return nil, err
		}
		u.Host = hostname
		artifactURL = u.String()
	}

	req, err := retryablehttp.NewRequest(http.MethodGet, artifactURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new request: %w", err)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download artifact, error: %w", err)
	}
	defer resp.Body.Close()

	// check response
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download artifact from %s, status: %s", artifactURL, resp.Status)
	}

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if artifact.Size != nil && resp.ContentLength != *artifact.Size {
		return nil, fmt.Errorf("expected artifact size %d, got %d", *artifact.Size, len(buf))
	}

	return bytes.NewBuffer(buf), nil
}

func (r *TerraformReconciler) shouldReconcile(terraform *infrav1.Terraform, source sourcev1.Source) (bool, string, time.Duration) {
	// reconcile if force is set
	if terraform.Spec.Force {
		return true, "force is enabled", 0
	}

	// reconcile when a *new* reconcile request annotation is present
	if requestedAt, ok := meta.ReconcileAnnotationValue(terraform.GetAnnotations()); ok {
		// we do SetLastHandledReconcileRequest in the defer func, so check it here
		if terraform.Status.GetLastHandledReconcileRequest() != requestedAt {
			return true, "new reconcile request annotation present", 0
		}
	}

	// reconcile if we have never planned
	if terraform.Status.LastPlanAt == nil {
		return true, "never planned before", 0
	}

	// reconcile if the object generation has changed
	if terraform.Status.ObservedGeneration != 0 && terraform.Generation != terraform.Status.ObservedGeneration {
		return true, "terraform generation has changed", 0
	}

	// reconcile if there is a pending plan or an apply should run
	if terraform.Status.Plan.Pending != "" || r.shouldApply(terraform) {
		return true, "pending plan or apply should run", 0
	}

	// reconcile if the source revision has changed since last attempt
	if source != nil && source.GetArtifact() != nil && terraform.Status.LastAttemptedRevision != source.GetArtifact().Revision {
		return true, "source revision has changed since last reconciliation attempt", 0
	}

	// reconcile if we were last blocked on a not ready dependency
	if conditions.HasAnyReason(terraform, meta.ReadyCondition, infrav1.DependencyNotReadyReason) {
		return true, "previously blocked on not ready dependency", 0
	}

	// reconcile if the last reconciliation failed or detected drift, and we are within the retry interval
	if (terraform.Status.ReconciliationFailures > 0 || driftUnresolved(terraform)) && terraform.Status.LastPlanAt != nil && !conditions.IsTrue(terraform, meta.ReadyCondition) {
		nextRetry := terraform.Status.LastPlanAt.Add(terraform.GetRetryInterval())
		retryIn := time.Until(nextRetry)
		if retryIn > 0 {
			return false, "retry interval has not elapsed since last failed reconciliation", retryIn
		}

		return true, "retry interval has elapsed since last failed reconciliation", 0
	}

	if terraform.Status.LastSuccessfulReconcileAt == nil {
		return true, "never successfuly reconciled before", 0
	}

	nextReconcile := terraform.Status.LastSuccessfulReconcileAt.Add(terraform.Spec.Interval.Duration)
	requeueAfter := time.Until(nextReconcile)
	if requeueAfter > 0 {
		return false, "interval has not elapsed since last successful reconciliation", requeueAfter
	}

	return true, "", 0
}

// driftUnresolved reports whether drift was detected after the last
// successful reconciliation. Drift is not counted as a failure, and later
// passes may replace the DriftDetected reason, so the timestamps decide.
func driftUnresolved(terraform *infrav1.Terraform) bool {
	drifted := terraform.Status.LastDriftDetectedAt
	succeeded := terraform.Status.LastSuccessfulReconcileAt
	return drifted != nil && (succeeded == nil || drifted.After(succeeded.Time))
}

func isBeingDeleted(terraform *infrav1.Terraform) bool {
	return !terraform.DeletionTimestamp.IsZero()
}

func (r *TerraformReconciler) checkDependencies(ctx context.Context, terraform *infrav1.Terraform, source sourcev1.Source) error {
	finalizerKey := infrav1.TFDependencyOfPrefix + terraform.GetName()

	for _, d := range terraform.Spec.DependsOn {
		dependencyName := types.NamespacedName{
			Namespace: d.Namespace,
			Name:      d.Name,
		}

		if dependencyName.Namespace == "" {
			dependencyName.Namespace = terraform.GetNamespace()
		}

		if r.NoCrossNamespaceRefs && dependencyName.Namespace != terraform.GetNamespace() {
			return acl.AccessDeniedError(
				fmt.Sprintf("cannot access %s/%s, cross-namespace references have been disabled", dependencyName.Namespace, dependencyName.Name),
			)
		}

		tDep := &infrav1.Terraform{}
		if err := r.Get(ctx, dependencyName, tDep); err != nil {
			return fmt.Errorf("unable to get '%s' dependency: %w", dependencyName, err)
		}

		// Check whether the dependent Terraform isn't being deleted, and then add a
		// a finalizer if it is missing
		if tDep.DeletionTimestamp.IsZero() && !controllerutil.ContainsFinalizer(tDep, finalizerKey) {
			patch := client.MergeFrom(tDep.DeepCopy())
			controllerutil.AddFinalizer(tDep, finalizerKey)
			if err := r.Patch(ctx, tDep, patch, client.FieldOwner(r.FieldManager)); err != nil {
				return fmt.Errorf("unable to add finalizer to '%s/%s' dependency: %w", dependencyName.Namespace, dependencyName.Name, err)
			}
		}

		// Check whether the dependent Terraform is ready
		if tDep.Generation != tDep.Status.ObservedGeneration || !conditions.IsTrue(tDep, meta.ReadyCondition) {
			return fmt.Errorf("dependency '%s' is not ready", dependencyName)
		}

		// Compare the source revisions
		sourceRevision := source.GetArtifact().Revision

		if tDep.Spec.SourceRef.Name == terraform.Spec.SourceRef.Name &&
			tDep.Spec.SourceRef.Namespace == terraform.Spec.SourceRef.Namespace &&
			tDep.Spec.SourceRef.Kind == terraform.Spec.SourceRef.Kind &&
			sourceRevision != tDep.Status.LastAppliedRevision &&
			sourceRevision != tDep.Status.LastPlannedRevision {
			return fmt.Errorf("dependency '%s' is not updated yet", dependencyName)
		}

		// If WriteOutputsToSecret is set, check whether we can access the secret
		if tDep.Spec.WriteOutputsToSecret != nil {
			outputSecretName := types.NamespacedName{
				Namespace: tDep.GetNamespace(),
				Name:      tDep.Spec.WriteOutputsToSecret.Name,
			}

			if err := r.Get(ctx, outputSecretName, &corev1.Secret{}); err != nil {
				return fmt.Errorf("dependency output secret: '%s' of '%s' is not ready yet", outputSecretName, dependencyName)
			}
		}
	}

	return nil
}

// invalidateStalePlan clears plan.pending when the pending plan is stale
// and reports whether any case fired. The cases are evaluated in order against
// the progressively mutated object; the caller patches once when this returns
// true.
func (r *TerraformReconciler) invalidateStalePlan(terraform *infrav1.Terraform, revision string) bool {
	cleared := false

	// case 1:
	// If revision is changed, and there's no intend to apply,
	// and has "replan" in the spec.approvePlan
	// we should clear the Pending Plan to trigger re-plan
	//
	if revision != terraform.Status.LastAttemptedRevision &&
		!r.shouldApply(terraform) &&
		strings.HasPrefix(terraform.Spec.ApprovePlan, "replan") &&
		strings.HasPrefix("re"+terraform.Status.Plan.Pending, terraform.Spec.ApprovePlan) {
		terraform.Status.Plan.Pending = ""
		cleared = true
	}

	// case 2:
	// if revision is changed, and planOnly is true,
	// we should clear the Pending Plan to trigger re-plan
	//
	if revision != terraform.Status.LastAttemptedRevision &&
		terraform.Spec.PlanOnly {
		terraform.Status.Plan.Pending = ""
		cleared = true
	}

	// case 3:
	// if revision is changed, there's a pending plan awaiting manual approval,
	// and the user hasn't approved or replanned, we should clear the Pending
	// Plan to trigger re-plan with the new source revision.
	//
	if revision != terraform.Status.LastAttemptedRevision &&
		terraform.Status.Plan.Pending != "" &&
		!r.shouldApply(terraform) {
		terraform.Status.Plan.Pending = ""
		cleared = true
	}

	// case 4:
	// detect when the Terraform spec has changed since we last generated
	// a plan (we see this as the LastPlannedGeneration will be different
	// than the current generation).
	if terraform.Status.LastPlannedGeneration != 0 &&
		terraform.Generation != terraform.Status.LastPlannedGeneration &&
		terraform.Status.Plan.Pending != "" &&
		!r.shouldApply(terraform) {
		terraform.Status.Plan.Pending = ""
		cleared = true
	}

	return cleared
}

// awaitingApproval reports whether reconciliation must stop to wait for
// a manual approval of the pending plan.
func (r *TerraformReconciler) awaitingApproval(terraform *infrav1.Terraform) bool {
	return terraform.Status.Plan.Pending != "" &&
		!r.forceOrAutoApply(terraform) &&
		!r.shouldApply(terraform)
}

// The functions below turn the outcome of a prerequisite check into status and
// a Reconcile result.

// dependantsOf returns the names of the objects that depend on obj, taken from
// its tf.dependency.of.* finalizers.
func dependantsOf(obj *infrav1.Terraform) []string {
	dependants := []string{}
	for _, finalizer := range obj.GetFinalizers() {
		if after, ok := strings.CutPrefix(finalizer, infrav1.TFDependencyOfPrefix); ok {
			dependants = append(dependants, after)
		}
	}
	return dependants
}

// deletionBlocked reports a deletion waiting for its dependants to go.
func (r *TerraformReconciler) deletionBlocked(ctx context.Context, patcher *patch.SerialPatcher, obj *infrav1.Terraform, dependants []string) (ctrl.Result, error) {
	msg := fmt.Sprintf("Deletion in progress, but blocked. Please delete %s to resume ...", strings.Join(dependants, ", "))
	infrav1.TerraformNotReady(obj, "", infrav1.DeletionBlockedByDependants, msg)
	if err := r.patchStatus(ctx, patcher, obj, "unable to update status"); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: obj.GetRetryInterval()}, nil
}

// sourceError reports a source that could not be resolved.
func (r *TerraformReconciler) sourceError(obj *infrav1.Terraform, err error) (ctrl.Result, error) {
	if acl.IsAccessDenied(err) {
		// The controller must restart or sourceRef to change
		// for this to be recoverable
		return r.accessDenied(obj, infrav1.ActionResolveSource, err)
	} else if apierrors.IsNotFound(err) {
		conditions.MarkStalled(obj, infrav1.ArtifactFailedReason, "%s", err)
		conditions.MarkFalse(obj, meta.ReadyCondition, infrav1.ArtifactFailedReason, "%s", err)
		conditions.Delete(obj, meta.ReconcilingCondition)
		r.Eventf(obj, nil, corev1.EventTypeWarning, infrav1.ArtifactFailedReason, infrav1.ActionResolveSource.String(), "%s", err.Error())

		return ctrl.Result{RequeueAfter: obj.GetRetryInterval()}, nil
	}

	msg := fmt.Sprintf("could not get Source object: %s", err.Error())
	conditions.MarkFalse(obj, meta.ReadyCondition, infrav1.ArtifactFailedReason, "%s", msg)
	conditions.Delete(obj, meta.ReconcilingCondition)
	return ctrl.Result{}, err
}

// accessDenied reports a cross-namespace reference the controller may not
// follow: Stalled and Ready False AccessDenied, a Warning event, and a
// terminal error. A later pass clears Stalled through
// MarkReconciling once the reference is fixed.
func (r *TerraformReconciler) accessDenied(obj *infrav1.Terraform, action infrav1.Action, err error) (ctrl.Result, error) {
	conditions.MarkStalled(obj, infrav1.AccessDeniedReason, "%s", err)
	conditions.MarkFalse(obj, meta.ReadyCondition, infrav1.AccessDeniedReason, "%s", err)
	conditions.Delete(obj, meta.ReconcilingCondition)
	r.Eventf(obj, nil, corev1.EventTypeWarning, infrav1.AccessDeniedReason, action.String(), "%s", err.Error())

	return ctrl.Result{}, reconcile.TerminalError(err)
}

// markInProgressIf sets Ready Unknown "Reconciliation in progress" when the
// Ready reason is one of reasons, clearing a prerequisite failure that no
// longer holds.
func markInProgressIf(obj *infrav1.Terraform, reasons ...string) {
	if conditions.HasAnyReason(obj, meta.ReadyCondition, reasons...) {
		conditions.MarkUnknown(obj, meta.ReadyCondition, meta.ProgressingReason, "Reconciliation in progress")
	}
}

// clearPrerequisiteFailure clears a Ready failure that an earlier pass left
// for a denied reference or an unready dependency, once the prerequisites
// hold.
func clearPrerequisiteFailure(obj *infrav1.Terraform) {
	markInProgressIf(obj, infrav1.AccessDeniedReason, infrav1.DependencyNotReadyReason)
}

// artifactMissing reports a source without an artifact.
func (r *TerraformReconciler) artifactMissing(ctx context.Context, obj *infrav1.Terraform) (ctrl.Result, error) {
	msg := "Source is not ready, artifact not found"
	conditions.MarkFalse(obj, meta.ReadyCondition, infrav1.ArtifactFailedReason, "%s", msg)
	conditions.Delete(obj, meta.ReconcilingCondition)

	ctrl.LoggerFrom(ctx).Info(msg)
	// do not requeue immediately, when the artifact is created the watcher should trigger a reconciliation
	return ctrl.Result{RequeueAfter: obj.GetRetryInterval()}, nil
}

// skipReconcile ends a pass that shouldReconcile skipped, requeueing when the
// next attempt is due.
func skipReconcile(ctx context.Context, obj *infrav1.Terraform, reason string, requeueAfter time.Duration) (ctrl.Result, error) {
	ctrl.LoggerFrom(ctx).Info("Skipping reconciliation",
		"reason", reason,
		"lastPlanAt", obj.Status.LastPlanAt,
		"lastSuccessfulReconcileAt", obj.Status.LastSuccessfulReconcileAt,
		"nextAttempt", time.Now().Add(requeueAfter),
		"requeueAfter", requeueAfter)

	// Clear the Reconciling condition as we are skipping it on this pass
	conditions.Delete(obj, meta.ReconcilingCondition)

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// dependenciesNotReady reports a dependsOn failure. A
// cross-namespace denial is terminal and stalled, like a denied source.
func (r *TerraformReconciler) dependenciesNotReady(ctx context.Context, patcher *patch.SerialPatcher, obj *infrav1.Terraform, revision string, err error) (ctrl.Result, error) {
	if acl.IsAccessDenied(err) {
		// Unlike a denied source, the source resolved here, so this revision
		// and generation count as attempted.
		obj.Status.ObservedGeneration = obj.Generation
		obj.Status.LastAttemptedRevision = revision

		// it won't succeed unless the dependsOn changes
		return r.accessDenied(obj, infrav1.ActionCheckDependencies, err)
	}

	infrav1.TerraformNotReady(obj, revision, infrav1.DependencyNotReadyReason, err.Error())
	if err := r.patchStatus(ctx, patcher, obj, "unable to update status for dependency not ready"); err != nil {
		return ctrl.Result{}, err
	}

	// we can't rely on exponential backoff because it will prolong the execution too much,
	// instead we requeue on a fix interval.
	ctrl.LoggerFrom(ctx).Info("Dependencies do not meet ready condition, retrying", "retryIn", obj.GetRetryInterval().String())
	r.Eventf(obj, nil, corev1.EventTypeNormal, infrav1.DependencyNotReadyReason, infrav1.ActionCheckDependencies.String(),
		"Dependencies do not meet ready condition, retrying in %s", obj.GetRetryInterval().String())

	return ctrl.Result{RequeueAfter: obj.GetRetryInterval()}, nil
}

// markReconciling marks obj Reconciling "Fulfilling prerequisites" and
// patches it.
func (r *TerraformReconciler) markReconciling(ctx context.Context, patcher *patch.SerialPatcher, obj *infrav1.Terraform) error {
	conditions.MarkReconciling(obj, meta.ProgressingReason, "Fulfilling prerequisites")
	return r.patchStatus(ctx, patcher, obj, "unable to record progress")
}

// markProgressing sets Ready Unknown "Reconciliation in progress" (or
// "Deletion in progress") and patches it, unless Ready is already Unknown, so
// a plan prompt stays visible, or reports a runner failure, so retrying the
// runner does not flap Ready.
func (r *TerraformReconciler) markProgressing(ctx context.Context, patcher *patch.SerialPatcher, obj *infrav1.Terraform) error {
	ready := apimeta.FindStatusCondition(obj.Status.Conditions, meta.ReadyCondition)
	if (ready != nil && ready.Status == metav1.ConditionUnknown) || runnerFailed(obj) {
		return nil
	}

	infrav1.TerraformProgressing(obj, progressingMessage(obj))
	return r.patchStatus(ctx, patcher, obj, "unable to update status before Terraform initialization")
}

// progressingMessage is the Ready message while a pass is in progress.
func progressingMessage(obj *infrav1.Terraform) string {
	if isBeingDeleted(obj) {
		return "Deletion in progress"
	}
	return "Reconciliation in progress"
}

// resetRetryAndStalePlan resets the retry count when the revision or spec
// changed and clears a stale pending plan, then patches
// if either happened.
func (r *TerraformReconciler) resetRetryAndStalePlan(ctx context.Context, patcher *patch.SerialPatcher, obj *infrav1.Terraform, revision string) error {
	revisionChanged := revision != obj.Status.LastAttemptedRevision
	generationChanges := obj.Generation != obj.Status.ObservedGeneration
	retryReset := revisionChanged || generationChanges
	if retryReset {
		ctrl.LoggerFrom(ctx).Info("Reset reconciliation failures count. Reason: resource changed")
		infrav1.TerraformResetRetry(obj)
	}

	// cases 1-4: clear a stale pending plan to trigger a re-plan
	stalePlanCleared := !isBeingDeleted(obj) && r.invalidateStalePlan(obj, revision)

	// Patch now rather than leaving it to a later patch, because users can
	// observe this intermediate state: it is what first writes "Reconciliation
	// in progress" after a cleared prerequisite failure, and it shows the
	// cleared plan before "Initializing".
	if retryReset || stalePlanCleared {
		return r.patchStatus(ctx, patcher, obj, "unable to update status after resetting retries or clearing a stale plan")
	}
	return nil
}
