/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/acl"
	"github.com/fluxcd/pkg/runtime/conditions"
	runtimeCtrl "github.com/fluxcd/pkg/runtime/controller"
	"github.com/fluxcd/pkg/runtime/dependency"
	"github.com/fluxcd/pkg/runtime/logger"
	"github.com/fluxcd/pkg/runtime/patch"
	"github.com/fluxcd/pkg/runtime/predicates"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	kuberecorder "k8s.io/client-go/tools/record"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/mtls"
)

// TerraformReconciler reconciles a Terraform object
type TerraformReconciler struct {
	client.Client
	kuberecorder.EventRecorder
	runtimeCtrl.Metrics

	httpClient        *retryablehttp.Client
	FieldManager      string
	patchOptions      []patch.Option
	requeueDependency time.Duration

	StatusPoller              *polling.StatusPoller
	Scheme                    *runtime.Scheme
	CertRotator               *mtls.CertRotator
	RunnerGRPCPort            int
	RunnerCreationTimeout     time.Duration
	RunnerGRPCMaxMessageSize  int
	AllowBreakTheGlass        bool
	ClusterDomain             string
	NoCrossNamespaceRefs      bool
	UsePodSubdomainResolution bool
	Clientset                 *kubernetes.Clientset
}

//+kubebuilder:rbac:groups=infra.contrib.fluxcd.io,resources=terraforms,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infra.contrib.fluxcd.io,resources=terraforms/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infra.contrib.fluxcd.io,resources=terraforms/finalizers,verbs=get;create;update;patch;delete
//+kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=buckets;gitrepositories;ocirepositories,verbs=get;list;watch
//+kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=buckets/status;gitrepositories/status;ocirepositories/status,verbs=get
//+kubebuilder:rbac:groups="",resources=configmaps;secrets;serviceaccounts,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Terraform object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *TerraformReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, retErr error) {
	startTime := time.Now()
	reconciliationLoopID := uuid.New().String()
	log := ctrl.LoggerFrom(ctx, "reconciliation-loop-id", reconciliationLoopID, "start-time", startTime)
	ctx = ctrl.LoggerInto(ctx, log)
	traceLog := log.V(logger.TraceLevel).WithValues("function", "TerraformReconciler.Reconcile")
	traceLog.Info("Reconcile Start")

	<-r.CertRotator.Ready

	traceLog.Info("Validate TLS Cert")
	if isCAValid, _ := r.CertRotator.IsCAValid(); isCAValid == false && r.CertRotator.TriggerCARotation != nil {
		traceLog.Info("TLS Cert invalid")
		readyCh := make(chan *mtls.TriggerResult)
		traceLog.Info("Trigger Cert Rotation")
		r.CertRotator.TriggerCARotation <- mtls.Trigger{Namespace: "", Ready: readyCh}
		traceLog.Info("Waiting for Cert Ready Signal")
		<-readyCh
		traceLog.Info("Ready Signal Received")
	}

	traceLog.Info("Fetch Terraform Resource", "namespacedName", req.NamespacedName)
	// Get the Terraform object
	terraform := &infrav1.Terraform{}
	if err := r.Get(ctx, req.NamespacedName, terraform); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.Info(fmt.Sprintf(">> Started Generation: %d", terraform.GetGeneration()))

	// Initialize the patch helper with the current version of the object.
	patchHelper := patch.NewSerialPatcher(terraform, r.Client)

	defer func() {
		if v, ok := meta.ReconcileAnnotationValue(terraform.GetAnnotations()); ok {
			terraform.Status.SetLastHandledReconcileRequest(v)
		}

		if conditions.IsTrue(terraform, meta.ReadyCondition) {
			terraform.Status.ObservedGeneration = terraform.Generation
		}

		if err := patchHelper.Patch(ctx, terraform, r.patchOptions...); err != nil {
			if !terraform.DeletionTimestamp.IsZero() {
				err = kerrors.FilterOut(err, func(e error) bool { return apierrors.IsNotFound(e) })
			}
			retErr = kerrors.NewAggregate([]error{retErr, err})
		}

		// Record the duration of the reconciliation.
		r.Metrics.RecordDuration(ctx, terraform, startTime)
	}()

	// Make sure the Finalizer exists
	traceLog.Info("Check Terraform resource for a finalizer")
	if !controllerutil.ContainsFinalizer(terraform, infrav1.TerraformFinalizer) {
		controllerutil.AddFinalizer(terraform, infrav1.TerraformFinalizer)
		return ctrl.Result{Requeue: true}, nil
	}

	// Return early if the Terraform is suspended.
	traceLog.Info("Check if the Terraform resource is suspened")
	if terraform.Spec.Suspend {
		log.Info("Reconciliation is suspended for this object")
		return ctrl.Result{}, nil
	}

	// Check whether we need to reconcile the release at this time
	if shouldReconcile, requeueAfter := r.shouldReconcile(terraform); !shouldReconcile {
		log.Info("Skipping reconciliation. Interval has not elapsed since last plan",
			"lastPlanAt", terraform.Status.LastPlanAt,
			"nextReconcile", terraform.Status.LastPlanAt.Add(terraform.Spec.Interval.Duration),
			"requeueAfter", requeueAfter)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	// Mark the resource as under reconciliation.
	conditions.MarkReconciling(terraform, meta.ProgressingReason, "Fulfilling prerequisites")
	if err := patchHelper.Patch(ctx, terraform, r.patchOptions...); err != nil {
		return ctrl.Result{}, err
	}

	// Examine if the object is under deletion
	if isBeingDeleted(terraform) {
		dependants := []string{}
		for _, finalizer := range terraform.GetFinalizers() {
			if strings.HasPrefix(finalizer, infrav1.TFDependencyOfPrefix) {
				dependants = append(dependants, strings.TrimPrefix(finalizer, infrav1.TFDependencyOfPrefix))
			}
		}

		if len(dependants) > 0 {
			msg := fmt.Sprintf("Deletion in progress, but blocked. Please delete %s to resume ...", strings.Join(dependants, ", "))
			terraform = infrav1.TerraformNotReady(terraform, "", infrav1.DeletionBlockedByDependants, msg)
			if err := patchHelper.Patch(ctx, terraform, r.patchOptions...); err != nil {
				log.Error(err, "unable to update status")
				return ctrl.Result{Requeue: true}, err
			}

			return ctrl.Result{RequeueAfter: terraform.GetRetryInterval()}, nil
		}
	}

	// resolve source reference
	log.Info("getting source")
	sourceObj, err := r.getSource(ctx, terraform)
	traceLog.Info("Did we get an error trying to get the source")
	if err != nil {
		traceLog.Info("Is the error a NotFound error")
		if acl.IsAccessDenied(err) {
			traceLog.Info("The cross-namespace Source was denied by reconciler.NoCrossNamespaceRefs")
			conditions.MarkStalled(terraform, infrav1.AccessDeniedReason, "%s", err)
			conditions.MarkFalse(terraform, meta.ReadyCondition, infrav1.AccessDeniedReason, "%s", err)
			conditions.Delete(terraform, meta.ReconcilingCondition)
			r.Eventf(terraform, corev1.EventTypeWarning, infrav1.AccessDeniedReason, err.Error())

			// The controller must restart or sourceRef to change
			// for this to be recoverable
			return ctrl.Result{}, reconcile.TerminalError(err)
		} else if apierrors.IsNotFound(err) {
			conditions.MarkStalled(terraform, infrav1.ArtifactFailedReason, "%s", err)
			conditions.MarkFalse(terraform, meta.ReadyCondition, infrav1.ArtifactFailedReason, "%s", err)
			conditions.Delete(terraform, meta.ReconcilingCondition)
			r.Eventf(terraform, corev1.EventTypeWarning, infrav1.ArtifactFailedReason, err.Error())

			return ctrl.Result{RequeueAfter: terraform.GetRetryInterval()}, nil
		}

		msg := fmt.Sprintf("could not get Source object: %s", err.Error())
		conditions.MarkFalse(terraform, meta.ReadyCondition, infrav1.ArtifactFailedReason, "%s", msg)
		conditions.Delete(terraform, meta.ReconcilingCondition)
		return ctrl.Result{}, err
	}

	if conditions.HasAnyReason(terraform, meta.ReadyCondition, infrav1.AccessDeniedReason, infrav1.ArtifactFailedReason) {
		conditions.MarkUnknown(terraform, meta.ReadyCondition, meta.ProgressingReason, "Reconciliation in progress")
	}

	// sourceObj does not exist, return early
	traceLog.Info("Check we have a source object")
	if sourceObj.GetArtifact() == nil {
		msg := "Source is not ready, artifact not found"
		conditions.MarkFalse(terraform, meta.ReadyCondition, infrav1.ArtifactFailedReason, "%s", msg)
		conditions.Delete(terraform, meta.ReconcilingCondition)

		log.Info(msg)
		// do not requeue immediately, when the artifact is created the watcher should trigger a reconciliation
		return ctrl.Result{RequeueAfter: terraform.GetRetryInterval()}, nil
	}

	if conditions.HasAnyReason(terraform, meta.ReadyCondition, infrav1.ArtifactFailedReason) {
		conditions.MarkUnknown(terraform, meta.ReadyCondition, meta.ProgressingReason, "Reconciliation in progress")
	}

	// check dependencies, if not being deleted
	if len(terraform.Spec.DependsOn) > 0 && !isBeingDeleted(terraform) {
		if err := r.checkDependencies(ctx, terraform, sourceObj); err != nil {
			if acl.IsAccessDenied(err) {
				traceLog.Info("The cross-namespace dependency was denied by reconciler.NoCrossNamespaceRefs")

				terraform = infrav1.TerraformNotReady(terraform, sourceObj.GetArtifact().Revision, infrav1.AccessDeniedReason, err.Error())
				if err := patchHelper.Patch(ctx, terraform, r.patchOptions...); err != nil {
					log.Error(err, "unable to update status for dependsOn access denied")
					return ctrl.Result{Requeue: true}, err
				}

				// don't requeue to retry; it won't succeed unless the dependsOn changes
				return ctrl.Result{}, nil
			}

			terraform = infrav1.TerraformNotReady(
				terraform, sourceObj.GetArtifact().Revision, infrav1.DependencyNotReadyReason, err.Error())

			if err := patchHelper.Patch(ctx, terraform, r.patchOptions...); err != nil {
				log.Error(err, "unable to update status for dependency not ready")
				return ctrl.Result{Requeue: true}, err
			}
			// we can't rely on exponential backoff because it will prolong the execution too much,
			// instead we requeue on a fix interval.
			msg := fmt.Sprintf("Dependencies do not meet ready condition, retrying in %s", terraform.GetRetryInterval().String())
			log.Info(msg)
			r.Eventf(terraform, corev1.EventTypeNormal, infrav1.DependencyNotReadyReason, msg)

			return ctrl.Result{RequeueAfter: terraform.GetRetryInterval()}, nil
		}
		log.Info("All dependencies are ready, proceeding with reconciliation")
	}

	if conditions.HasAnyReason(terraform, meta.ReadyCondition, infrav1.AccessDeniedReason, infrav1.DependencyNotReadyReason) {
		conditions.MarkUnknown(terraform, meta.ReadyCondition, meta.ProgressingReason, "Reconciliation in progress")
	}

	// Skip update the status if the ready condition is still unknown
	// so that the Plan prompt is still shown.
	ready := apimeta.FindStatusCondition(terraform.Status.Conditions, meta.ReadyCondition)
	log.Info("before lookup runner: checking ready condition", "ready", ready)
	if ready == nil || ready.Status != metav1.ConditionUnknown {

		msg := "Reconciliation in progress"
		if isBeingDeleted(terraform) {
			msg = "Deletion in progress"
		}

		log.Info("before lookup runner: updating status", "ready", ready)
		terraform = infrav1.TerraformProgressing(terraform, msg)
		if err := patchHelper.Patch(ctx, terraform, r.patchOptions...); err != nil {
			log.Error(err, "unable to update status before Terraform initialization")
			return ctrl.Result{Requeue: true}, err
		}
		log.Info("before lookup runner: updated status", "ready", ready)
	}

	// Reset retry count if necessary.
	revisionChanged := sourceObj.GetArtifact().Revision != terraform.Status.LastAttemptedRevision
	generationChanges := terraform.Generation != terraform.Status.ObservedGeneration
	if revisionChanged || generationChanges {
		log.Info("Reset reconciliation failures count. Reason: resource changed")
		terraform = infrav1.TerraformResetRetry(terraform)
		if err := patchHelper.Patch(ctx, terraform, r.patchOptions...); err != nil {
			log.Error(err, "unable to update status after planning")
			return ctrl.Result{Requeue: true}, err
		}
	}

	if !isBeingDeleted(terraform) {
		// case 1:
		// If revision is changed, and there's no intend to apply,
		// and has "replan" in the spec.approvePlan
		// we should clear the Pending Plan to trigger re-plan
		//
		traceLog.Info("Check artifact revision and if we shouldApply")
		if sourceObj.GetArtifact().Revision != terraform.Status.LastAttemptedRevision &&
			!r.shouldApply(terraform) &&
			strings.HasPrefix(terraform.Spec.ApprovePlan, "replan") &&
			strings.HasPrefix("re"+terraform.Status.Plan.Pending, terraform.Spec.ApprovePlan) {
			traceLog.Info("Update the status of the Terraform resource")
			terraform.Status.Plan.Pending = ""
			if err := patchHelper.Patch(ctx, terraform, r.patchOptions...); err != nil {
				log.Error(err, "unable to update status to clear pending plan (revision != last attempted)")
				return ctrl.Result{Requeue: true}, err
			}
		}

		// case 2:
		// if revision is changed, and planOnly is true,
		// we should clear the Pending Plan to trigger re-plan
		//
		if sourceObj.GetArtifact().Revision != terraform.Status.LastAttemptedRevision &&
			terraform.Spec.PlanOnly {
			traceLog.Info("Update the status of the Terraform resource")
			terraform.Status.Plan.Pending = ""
			if err := patchHelper.Patch(ctx, terraform, r.patchOptions...); err != nil {
				log.Error(err, "unable to update status to clear pending plan (revision != last attempted)")
				return ctrl.Result{Requeue: true}, err
			}
		}

		// case 3:
		// return early if it's manually mode and pending
		//
		traceLog.Info("Check for pending plan, forceOrAutoApply and shouldApply")
		if terraform.Status.Plan.Pending != "" &&
			!r.forceOrAutoApply(terraform) &&
			!r.shouldApply(terraform) {
			log.Info("reconciliation is stopped to wait for a manual approve")
			return ctrl.Result{}, nil
		}
	}

	// Create Runner Pod.
	// Wait for the Runner Pod to start.
	traceLog.Info("Fetch/Create Runner pod for this Terraform resource")
	runnerClient, closeConn, err := r.LookupOrCreateRunner(ctx, terraform, sourceObj.GetArtifact().Revision)
	if err != nil {
		log.Error(err, "unable to lookup or create runner")
		if closeConn != nil {
			if err := closeConn(); err != nil {
				log.Error(err, "unable to close connection")
			}
		}
		return ctrl.Result{}, err
	}
	log.Info("runner is running")

	traceLog.Info("Defer function to handle clean up")
	defer func(ctx context.Context, cli client.Client, terraform *infrav1.Terraform) {
		traceLog.Info("Check for closeConn function")
		// make sure defer does not affect the return value
		if closeConn != nil {
			traceLog.Info("Call closeConn function")
			if err := closeConn(); err != nil {
				log.Error(err, "unable to close connection")
			}
		}

		traceLog.Info("Check if we're running an insecure local runner")
		if os.Getenv("INSECURE_LOCAL_RUNNER") == "1" {
			// nothing to delete
			log.Info("insecure local runner")
			return
		}

		traceLog.Info("Check if we need to clean up the Runner pod")
		if terraform.Spec.GetAlwaysCleanupRunnerPod() {
			// wait for runner pod complete termination
			var (
				interval = time.Second * 5
				timeout  = time.Second * 120
			)
			traceLog.Info("Poll function that will clean up the Runner pod")
			err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
				traceLog.Info("Get the Runner pod")
				var runnerPod corev1.Pod
				err := r.Get(ctx, getRunnerPodObjectKey(terraform), &runnerPod)

				traceLog.Info("If not found nothing to do")
				if err != nil && apierrors.IsNotFound(err) {
					traceLog.Info("Runner pod not running, work complete")
					return true, nil
				}

				traceLog.Info("Attempt to delete the Runner pod")
				if err := cli.Delete(ctx, &runnerPod,
					client.GracePeriodSeconds(1), // force kill = 1 second
					client.PropagationPolicy(metav1.DeletePropagationForeground),
				); err != nil {
					log.Error(err, "unable to delete pod")
					return false, nil
				}

				traceLog.Info("Unable to delete the Runner pod, return false and err to try again")
				return false, err
			})

			traceLog.Info("If err != nil we hit an error deleting the Runner pod")
			if err != nil {
				log.Error(fmt.Errorf("failed waiting for the terminating runner pod: %v", err), "error in polling")
			}
		}
	}(ctx, r.Client, terraform)

	// Examine if the object is under deletion
	traceLog.Info("Check for deletion timestamp to finalize")
	if !terraform.ObjectMeta.DeletionTimestamp.IsZero() {
		traceLog.Info("Calling finalize function")
		if terraform, result, err := r.finalize(ctx, patchHelper, terraform, runnerClient, sourceObj, reconciliationLoopID); err != nil {
			traceLog.Info("Patch the status of the Terraform resource")
			if patchErr := patchHelper.Patch(ctx, terraform, r.patchOptions...); patchErr != nil {
				log.Error(patchErr, "unable to update status after the finalize is complete")
				return ctrl.Result{Requeue: true}, patchErr
			}
			return result, err
		}
	}

	if !terraform.ShouldRetry() {
		// `ShouldRetry` will return true if .Spec.Remediation is nil.
		// The code doesn't reach this block if .Spec.Remediation is nil.
		log.Info(fmt.Sprintf(
			"Resource reached maximum number of retries (%d/%d). Generation: %d",
			terraform.GetReconciliationFailures(),
			terraform.Spec.Remediation.Retries,
			terraform.GetGeneration(),
		))

		terraform = infrav1.TerraformReachedLimit(terraform)
		conditions.Delete(terraform, meta.ReconcilingCondition)

		traceLog.Info("Patch the status of the Terraform resource")
		if err := patchHelper.Patch(ctx, terraform, r.patchOptions...); err != nil {
			log.Error(err, "unable to update status after the reconciliation is complete")
			return ctrl.Result{Requeue: true}, err
		}

		return ctrl.Result{Requeue: false}, nil
	}

	// reconcile Terraform by applying the latest revision
	traceLog.Info("Run reconcile for the Terraform resource")
	reconciledTerraform, reconcileErr := r.reconcile(ctx, patchHelper, runnerClient, terraform.DeepCopy(), sourceObj, reconciliationLoopID)

	// Check remediation.
	if reconcileErr == nil {
		log.Info("Reset reconciliation failures count. Reason: successful reconciliation")
		terraform = infrav1.TerraformResetRetry(reconciledTerraform)
	} else {
		terraform = reconciledTerraform
		terraform.IncrementReconciliationFailures()
	}

	traceLog.Info("Patch the status of the Terraform resource")
	if err := patchHelper.Patch(ctx, terraform, r.patchOptions...); err != nil {
		log.Error(err, "unable to update status after the reconciliation is complete")
		return ctrl.Result{Requeue: true}, err
	}

	traceLog.Info("Check for reconciliation errors")
	if reconcileErr != nil && reconcileErr.Error() == infrav1.DriftDetectedReason {
		log.Error(reconcileErr, fmt.Sprintf("Drift detected after %s, next try in %s",
			time.Since(startTime).String(),
			terraform.GetRetryInterval().String()),
			"revision",
			sourceObj.GetArtifact().Revision)

		return ctrl.Result{RequeueAfter: terraform.GetRetryInterval()}, nil
	} else if reconcileErr != nil {
		// broadcast the reconciliation failure and requeue at the specified retry interval
		log.Error(reconcileErr, fmt.Sprintf("Reconciliation failed after %s, next try in %s",
			time.Since(startTime).String(),
			terraform.GetRetryInterval().String()),
			"revision",
			sourceObj.GetArtifact().Revision)
		traceLog.Info("Record an event for the failure")
		r.Eventf(terraform, corev1.EventTypeWarning, infrav1.ReconciliationFailureReason, reconcileErr.Error())

		if terraform.Spec.Remediation != nil {
			log.Info(fmt.Sprintf(
				"Reconciliation failed, retry (%d/%d) after %s. Generation: %d",
				terraform.GetReconciliationFailures(),
				terraform.Spec.Remediation.Retries,
				terraform.GetRetryInterval(),
				terraform.GetGeneration(),
			))
		} else {
			log.Info(fmt.Sprintf(
				"Reconciliation failed, retry after %s. Generation: %d",
				terraform.GetRetryInterval(),
				terraform.GetGeneration(),
			))
		}

		return ctrl.Result{RequeueAfter: terraform.GetRetryInterval()}, nil
	}

	log.Info(fmt.Sprintf("Reconciliation completed. Generation: %d", terraform.GetGeneration()))

	traceLog.Info("Check for pending plan and forceOrAutoApply")
	if terraform.Status.Plan.Pending != "" && !r.forceOrAutoApply(terraform) {
		log.Info("Reconciliation is stopped to wait for manual operations")
		return ctrl.Result{}, nil
	}

	// next reconcile is .Spec.Interval in the future
	log.Info("requeue after interval", "interval", terraform.Spec.Interval.Duration.String())
	return ctrl.Result{RequeueAfter: terraform.Spec.Interval.Duration}, nil
}

func (r *TerraformReconciler) shouldReconcile(terraform *infrav1.Terraform) (bool, time.Duration) {
	// Always plan is .Spec.Force is true
	if terraform.Spec.Force {
		return true, 0
	}

	// Do not delay if being deleted
	if isBeingDeleted(terraform) {
		return true, 0
	}

	// If we have never planned, we should continue
	if terraform.Status.LastPlanAt == nil {
		return true, 0
	}

	// If the generation has changed, we should continue
	if terraform.Generation != terraform.Status.ObservedGeneration {
		return true, 0
	}

	// Do not delay if there is a pending plan or an apply should run.
	if terraform.Status.Plan.Pending != "" || r.shouldApply(terraform) {
		return true, 0
	}

	nextReconcile := terraform.Status.LastPlanAt.Add(terraform.Spec.Interval.Duration)
	requeueAfter := time.Until(nextReconcile)
	if requeueAfter > 0 {
		return false, requeueAfter
	}

	return true, 0
}

func isBeingDeleted(terraform *infrav1.Terraform) bool {
	return !terraform.ObjectMeta.DeletionTimestamp.IsZero()
}

// SetupWithManager sets up the controller with the Manager.
func (r *TerraformReconciler) SetupWithManager(mgr ctrl.Manager, maxConcurrentReconciles int, httpRetry int) error {
	// Index the Terraforms by the GitRepository references they (may) point at.
	if err := mgr.GetCache().IndexField(context.TODO(), &infrav1.Terraform{}, infrav1.GitRepositoryIndexKey,
		r.IndexBy(sourcev1.GitRepositoryKind)); err != nil {
		return fmt.Errorf("failed setting index fields: %w", err)
	}

	// Index the Terraforms by the Bucket references they (may) point at.
	if err := mgr.GetCache().IndexField(context.TODO(), &infrav1.Terraform{}, infrav1.BucketIndexKey,
		r.IndexBy(sourcev1b2.BucketKind)); err != nil {
		return fmt.Errorf("failed setting index fields: %w", err)
	}

	// Index the Terraforms by the OCIRepository references they (may) point at.
	if err := mgr.GetCache().IndexField(context.TODO(), &infrav1.Terraform{}, infrav1.OCIRepositoryIndexKey,
		r.IndexBy(sourcev1b2.OCIRepositoryKind)); err != nil {
		return fmt.Errorf("failed setting index fields: %w", err)
	}

	// Configure the retryable http client used for fetching artifacts.
	// By default, it retries 10 times within a 3.5 minutes window.
	httpClient := retryablehttp.NewClient()
	httpClient.RetryWaitMin = 5 * time.Second
	httpClient.RetryWaitMax = 30 * time.Second
	httpClient.RetryMax = httpRetry
	httpClient.Logger = nil
	r.httpClient = httpClient
	r.patchOptions = []patch.Option{patch.WithOwnedConditions{Conditions: infrav1.OwnedConditions}, patch.WithFieldOwner(r.FieldManager)}
	r.requeueDependency = 30 * time.Second
	recoverPanic := true

	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.Terraform{}, builder.WithPredicates(
			predicate.Or(predicate.GenerationChangedPredicate{}, predicates.ReconcileRequestedPredicate{}),
		)).
		Watches(
			&sourcev1.GitRepository{},
			handler.EnqueueRequestsFromMapFunc(r.requestsForRevisionChangeOf(infrav1.GitRepositoryIndexKey)),
			builder.WithPredicates(SourceRevisionChangePredicate{}),
		).
		Watches(
			&sourcev1b2.Bucket{},
			handler.EnqueueRequestsFromMapFunc(r.requestsForRevisionChangeOf(infrav1.BucketIndexKey)),
			builder.WithPredicates(SourceRevisionChangePredicate{}),
		).
		Watches(
			&sourcev1b2.OCIRepository{},
			handler.EnqueueRequestsFromMapFunc(r.requestsForRevisionChangeOf(infrav1.OCIRepositoryIndexKey)),
			builder.WithPredicates(SourceRevisionChangePredicate{}),
		).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &infrav1.Terraform{}, handler.OnlyControllerOwner()),
			builder.WithPredicates(SecretDeletePredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: maxConcurrentReconciles,
			RecoverPanic:            &recoverPanic,
		}).
		Complete(r)
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
		if tDep.ObjectMeta.DeletionTimestamp.IsZero() && !controllerutil.ContainsFinalizer(tDep, finalizerKey) {
			patch := client.MergeFrom(tDep.DeepCopy())
			controllerutil.AddFinalizer(tDep, finalizerKey)
			if err := r.Patch(context.Background(), tDep, patch, client.FieldOwner(r.FieldManager)); err != nil {
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

			if err := r.Get(context.Background(), outputSecretName, &corev1.Secret{}); err != nil {
				return fmt.Errorf("dependency output secret: '%s' of '%s' is not ready yet", outputSecretName, dependencyName)
			}
		}
	}

	return nil
}

func (r *TerraformReconciler) requestsForRevisionChangeOf(indexKey string) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		log := ctrl.LoggerFrom(ctx)
		repo, ok := obj.(interface {
			GetArtifact() *meta.Artifact
		})
		if !ok {
			log.Error(fmt.Errorf("expected an object conformed with GetArtifact() method, but got a %T", obj), "failed to get reconcile requests for revision change")
			return nil
		}
		// If we do not have an artifact, we have no requests to make
		if repo.GetArtifact() == nil {
			return nil
		}

		var list infrav1.TerraformList
		if err := r.List(ctx, &list, client.MatchingFields{
			indexKey: client.ObjectKeyFromObject(obj).String(),
		}); err != nil {
			log.Error(err, "failed to list objects for revision change")
			return nil
		}
		var dd []dependency.Dependent
		for _, d := range list.Items {
			// If the revision of the artifact equals to the last attempted revision,
			// we should not make a request for this Terraform
			if repo.GetArtifact().Revision == d.Status.LastAttemptedRevision {
				continue
			}
			dd = append(dd, d.DeepCopy())
		}
		sorted, err := dependency.Sort(dd)
		if err != nil {
			log.Error(err, "failed to sort dependencies for revision change")
			return nil
		}
		reqs := make([]reconcile.Request, len(sorted))
		for i, t := range sorted {
			reqs[i].NamespacedName.Name = t.Name
			reqs[i].NamespacedName.Namespace = t.Namespace
		}
		return reqs
	}

}

func (r *TerraformReconciler) getSource(ctx context.Context, terraform *infrav1.Terraform) (sourcev1.Source, error) {
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
		err := r.Client.Get(ctx, sourceReference, &repository)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return sourceObj, err
			}
			return sourceObj, fmt.Errorf("unable to get source '%s': %w", sourceReference, err)
		}
		sourceObj = &repository
	case sourcev1b2.BucketKind:
		var bucket sourcev1b2.Bucket
		err := r.Client.Get(ctx, sourceReference, &bucket)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return sourceObj, err
			}
			return sourceObj, fmt.Errorf("unable to get source '%s': %w", sourceReference, err)
		}
		sourceObj = &bucket
	case sourcev1b2.OCIRepositoryKind:
		var repository sourcev1b2.OCIRepository
		err := r.Client.Get(ctx, sourceReference, &repository)
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

func (r *TerraformReconciler) IndexBy(kind string) func(o client.Object) []string {
	return func(o client.Object) []string {
		terraform, ok := o.(*infrav1.Terraform)
		if !ok {
			panic(fmt.Sprintf("Expected a Terraform, got %T", o))
		}

		if terraform.Spec.SourceRef.Kind == kind {
			namespace := terraform.GetNamespace()
			if terraform.Spec.SourceRef.Namespace != "" {
				namespace = terraform.Spec.SourceRef.Namespace
			}
			return []string{fmt.Sprintf("%s/%s", namespace, terraform.Spec.SourceRef.Name)}
		}

		return nil
	}
}
