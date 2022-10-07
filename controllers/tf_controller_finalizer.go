package controllers

import (
	"context"
	"fmt"
	"github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/weaveworks/tf-controller/api/v1alpha1"
	"github.com/weaveworks/tf-controller/runner"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *TerraformReconciler) finalize(ctx context.Context, terraform v1alpha1.Terraform, runnerClient runner.RunnerClient, sourceObj v1beta2.Source) (controllerruntime.Result, error) {
	log := controllerruntime.LoggerFrom(ctx)
	objectKey := types.NamespacedName{Namespace: terraform.Namespace, Name: terraform.Name}

	// TODO how to completely delete without planning?
	if terraform.Spec.DestroyResourcesOnDeletion {
		// TODO There's a case of sourceObj got deleted before finalize is called.
		revision := sourceObj.GetArtifact().Revision
		terraform, tfInstance, tmpDir, err := r.setupTerraform(ctx, runnerClient, terraform, sourceObj, revision, objectKey)

		defer func() {
			cleanupDirReply, err := runnerClient.CleanupDir(ctx, &runner.CleanupDirRequest{TmpDir: tmpDir})
			if err != nil {
				log.Error(err, "clean up error")
			}
			if cleanupDirReply != nil {
				log.Info(fmt.Sprintf("clean up dir: %s", cleanupDirReply.Message))
			}
		}()

		if err != nil {
			return controllerruntime.Result{Requeue: true}, err
		}

		// This will create the "destroy" plan because deletion timestamp is set.
		terraform, err = r.plan(ctx, terraform, tfInstance, runnerClient, revision)
		if err != nil {
			return controllerruntime.Result{Requeue: true}, err
		}

		if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
			log.Error(err, "unable to update status after planing")
			return controllerruntime.Result{Requeue: true}, err
		}

		terraform, err = r.apply(ctx, terraform, tfInstance, runnerClient, revision)
		if err != nil {
			return controllerruntime.Result{Requeue: true}, err
		}

		if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
			log.Error(err, "unable to update status after applying")
			return controllerruntime.Result{Requeue: true}, err
		}

		if err == nil {
			log.Info("finalizing destroyResourcesOnDeletion: ok")
		}
	}

	outputSecretName := ""
	hasSpecifiedOutputSecret := terraform.Spec.WriteOutputsToSecret != nil && terraform.Spec.WriteOutputsToSecret.Name != ""
	if hasSpecifiedOutputSecret {
		outputSecretName = terraform.Spec.WriteOutputsToSecret.Name
	}

	finalizeSecretsReply, err := runnerClient.FinalizeSecrets(ctx, &runner.FinalizeSecretsRequest{
		Namespace:                terraform.Namespace,
		Name:                     terraform.Name,
		HasSpecifiedOutputSecret: hasSpecifiedOutputSecret,
		OutputSecretName:         outputSecretName,
	})
	if err != nil {
		if e, ok := status.FromError(err); ok {
			switch e.Code() {
			case codes.Internal:
				// transient error
				return controllerruntime.Result{Requeue: true}, err
			case codes.NotFound:
				// do nothing, fall through
			}
		}
	}

	if err == nil {
		log.Info(fmt.Sprintf("finalizing secrets: %s", finalizeSecretsReply.Message))
	}

	// Record deleted status
	r.recordReadinessMetric(ctx, terraform)

	if err := r.Get(ctx, objectKey, &terraform); err != nil {
		return controllerruntime.Result{}, err
	}

	// Remove our finalizer from the list and update it
	controllerutil.RemoveFinalizer(&terraform, v1alpha1.TerraformFinalizer)
	if err := r.Update(ctx, &terraform); err != nil {
		return controllerruntime.Result{}, err
	}

	// Stop reconciliation as the object is being deleted
	return controllerruntime.Result{}, nil
}
