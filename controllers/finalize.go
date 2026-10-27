package controllers

import (
	"context"

	"github.com/fluxcd/pkg/runtime/patch"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"
)

// reconcileDelete handles an object being deleted. Reconcile calls it before
// the finalizer and suspend checks, so a suspended object can still be
// deleted. The pass always ends here: the normal pipeline never runs on a
// deleting object.
func (r *TerraformReconciler) reconcileDelete(ctx context.Context, patcher *patch.SerialPatcher, obj *infrav1.Terraform, reconcileID string) (ctrl.Result, error) {
	// Nothing of ours to clean up: never initialised, or already finalized.
	if !controllerutil.ContainsFinalizer(obj, infrav1.TerraformFinalizer) {
		return ctrl.Result{}, nil
	}

	// Show that this pass is checking prerequisites.
	if err := r.markReconciling(ctx, patcher, obj); err != nil {
		return ctrl.Result{}, err
	}

	// Wait while other Terraforms still depend on this one.
	if dependants := dependantsOf(obj); len(dependants) > 0 {
		return r.deletionBlocked(ctx, patcher, obj, dependants)
	}

	// A suspended object is deleted without running destroy.
	destroy := obj.Spec.DestroyResourcesOnDeletion && !obj.Spec.Suspend

	// Only destroy needs the source code, so the source is resolved, and can
	// block deletion, only when destroy will run.
	var src sourcev1.Source
	revision := ""
	if destroy {
		resolved, ok, res, err := r.resolveArtifact(ctx, obj)
		if !ok {
			return res, err
		}
		src, revision = resolved, resolved.GetArtifact().Revision
	}

	// Show progress and reset the retry count when the source or spec
	// changed. Without a source, only a spec change resets it.
	clearPrerequisiteFailure(obj)
	if err := r.markProgressing(ctx, patcher, obj); err != nil {
		return ctrl.Result{}, err
	}
	retryRevision := revision
	if !destroy {
		retryRevision = obj.Status.LastAttemptedRevision
	}
	if err := r.resetRetryAndStalePlan(ctx, patcher, obj, retryRevision); err != nil {
		return ctrl.Result{}, err
	}

	// The runner is needed in every case, to finalize its secrets. Without a
	// source revision the runner pod's instance label falls back to the
	// status revisions (see runnerPodInstance). Closing the session cleans up
	// the runner.
	sess, err := r.openSession(ctx, patcher, obj, revision, reconcileID)
	if err != nil {
		return r.runnerError(ctx, obj, err)
	}
	defer sess.Close(ctx)

	return r.finalize(ctx, sess, obj, src, destroy)
}

// finalize runs the runner side of a deletion: destroy when
// destroy is true (setup, destroy plan, apply unless there is nothing to
// destroy), finalize the runner's secrets, and remove the dependant and own
// finalizers. On error it patches status before returning.
func (r *TerraformReconciler) finalize(ctx context.Context, s *runnerSession, obj *infrav1.Terraform, src sourcev1.Source, destroy bool) (result ctrl.Result, retErr error) {
	log := ctrl.LoggerFrom(ctx)

	// On failure, patch status now rather than after the runner cleanup, keeping the existing patch timing.
	defer func() {
		if retErr == nil {
			return
		}
		if err := r.patchStatus(ctx, s.patcher, obj, "unable to update status after the finalize is complete"); err != nil {
			result, retErr = ctrl.Result{}, err
		}
	}()

	if destroy {
		// Remove the working directory when finalize ends, even if setup fails
		// after creating it.
		defer s.cleanupDir(ctx)
		if err := r.setup(ctx, s, obj, src); err != nil {
			return ctrl.Result{}, err
		}

		// This will create the "destroy" plan because deletion timestamp is set.
		if err := r.plan(ctx, s, obj); err != nil {
			return ctrl.Result{}, err
		}

		if !thereIsNothingToDestroy(obj) {
			if err := r.apply(ctx, s, obj); err != nil {
				return ctrl.Result{}, err
			}
			log.Info("finalizing destroyResourcesOnDeletion: ok")
		}
	} else if obj.Spec.DestroyResourcesOnDeletion {
		// Destroy was requested but the object is suspended. Say so, because
		// the object's status is deleted along with it.
		msg := "destroy skipped: the object is suspended, so its resources are left in place"
		log.Info(msg)
		r.Eventf(obj, nil, corev1.EventTypeNormal, infrav1.DestroySkippedReason, infrav1.ActionDestroy.String(), "%s", msg)
	}

	objectKey := types.NamespacedName{Namespace: obj.Namespace, Name: obj.Name}

	outputSecretName := ""
	hasSpecifiedOutputSecret := obj.Spec.WriteOutputsToSecret != nil && obj.Spec.WriteOutputsToSecret.Name != ""
	if hasSpecifiedOutputSecret {
		outputSecretName = obj.Spec.WriteOutputsToSecret.Name
	}

	finalizeSecretsReply, err := s.client.FinalizeSecrets(ctx, &runner.FinalizeSecretsRequest{
		Namespace:                obj.Namespace,
		Name:                     obj.Name,
		Workspace:                obj.WorkspaceName(),
		HasSpecifiedOutputSecret: hasSpecifiedOutputSecret,
		OutputSecretName:         outputSecretName,
	})
	if err != nil {
		// Internal is transient and Unavailable means the runner could not be
		// reached; both block deletion, as removing the finalizer now would leave
		// the plan secrets behind. NotFound means there was nothing to delete.
		// Anything else, including a non-gRPC error (code Unknown), is reported
		// and deletion proceeds.
		switch status.Code(err) {
		case codes.Internal, codes.Unavailable:
			return ctrl.Result{}, err
		case codes.NotFound:
			// do nothing, fall through
		default:
			r.Eventf(obj, nil, corev1.EventTypeWarning, infrav1.FinalizeSecretsFailedReason, infrav1.ActionFinalizeSecrets.String(), "%s", err.Error())
		}
	}

	if err == nil {
		log.Info("finalizing secrets", "message", finalizeSecretsReply.Message)
	}

	if err := r.Get(ctx, objectKey, obj); err != nil {
		return ctrl.Result{}, err
	}

	// Remove the dependant finalizer from every dependency
	dependantFinalizer := infrav1.TFDependencyOfPrefix + obj.GetName()
	for _, d := range obj.Spec.DependsOn {
		if d.Namespace == "" {
			d.Namespace = obj.GetNamespace()
		}
		dName := types.NamespacedName{
			Namespace: d.Namespace,
			Name:      d.Name,
		}
		var tf infrav1.Terraform
		err := r.Get(ctx, dName, &tf)
		if apierrors.IsNotFound(err) {
			// The target is gone, so there is no finalizer to release.
			continue
		}
		if err != nil {
			return ctrl.Result{}, err
		}

		// remove the finalizer from the dependency
		if controllerutil.ContainsFinalizer(&tf, dependantFinalizer) {
			patch := client.MergeFrom(tf.DeepCopy())
			controllerutil.RemoveFinalizer(&tf, dependantFinalizer)
			if err := r.Patch(ctx, &tf, patch, client.FieldOwner(r.FieldManager)); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	patch := client.MergeFrom(obj.DeepCopy())
	controllerutil.RemoveFinalizer(obj, infrav1.TerraformFinalizer)
	if err := r.Patch(ctx, obj, patch, client.FieldOwner(r.FieldManager)); err != nil {
		return ctrl.Result{}, err
	}

	// The object is gone or going, so there is nothing to requeue.
	return ctrl.Result{}, nil
}

func thereIsNothingToDestroy(obj *infrav1.Terraform) bool {
	// find condition with type "Plan"
	for _, c := range obj.Status.Conditions {
		if c.Type == infrav1.ConditionTypePlan {
			if c.Status == metav1.ConditionFalse &&
				c.Reason == infrav1.PlannedNoChangesReason &&
				c.Message == "No objects need to be destroyed" {
				return true
			}
		}
	}
	return false
}
