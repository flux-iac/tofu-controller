package controllers

import (
	"context"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"

	"github.com/fluxcd/pkg/runtime/logger"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
	"github.com/weaveworks/tf-controller/runner"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/types"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *TerraformReconciler) finalize(ctx context.Context, terraform infrav1.Terraform, runnerClient runner.RunnerClient, sourceObj sourcev1.Source, reconciliationLoopID string) (infrav1.Terraform, controllerruntime.Result, error) {
	log := controllerruntime.LoggerFrom(ctx)
	traceLog := log.V(logger.TraceLevel).WithValues("function", "TerraformReconciler.finalize")
	objectKey := types.NamespacedName{Namespace: terraform.Namespace, Name: terraform.Name}

	// TODO how to completely delete without planning?
	traceLog.Info("Check if we need to Destroy on Delete")
	if terraform.Spec.DestroyResourcesOnDeletion {

		for _, finalizer := range terraform.GetFinalizers() {
			if strings.HasPrefix(finalizer, infrav1.TFDependencyOfPrefix) {
				log.Info("waiting for a dependant to be deleted", "dependant", finalizer)
				msg := fmt.Sprintf("waiting for a dependant to be deleted: %s", strings.TrimPrefix(finalizer, infrav1.TFDependencyOfPrefix))
				terraform = infrav1.TerraformNotReady(terraform, "", infrav1.DeletionBlockedByDependants, msg)
				if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
					log.Error(err, "unable to update status for source not found")
					return terraform, controllerruntime.Result{Requeue: true}, nil
				}

				return terraform, controllerruntime.Result{Requeue: true}, nil
			}
		}

		// TODO There's a case of sourceObj got deleted before finalize is called.
		revision := sourceObj.GetArtifact().Revision
		traceLog.Info("Setup the terraform instance")
		terraform, tfInstance, tmpDir, err := r.setupTerraform(ctx, runnerClient, terraform, sourceObj, revision, objectKey, reconciliationLoopID)

		traceLog.Info("Defer function for cleanup")
		defer func() {
			traceLog.Info("Run CleanupDir")
			cleanupDirReply, err := runnerClient.CleanupDir(ctx, &runner.CleanupDirRequest{TmpDir: tmpDir})
			traceLog.Info("Check for error")
			if err != nil {
				log.Error(err, "clean up error")
			}
			traceLog.Info("Check for cleanupDirReply")
			if cleanupDirReply != nil {
				log.Info(fmt.Sprintf("clean up dir: %s", cleanupDirReply.Message))
			}
		}()

		traceLog.Info("Check for error")
		if err != nil {
			traceLog.Error(err, "Error, requeue job")
			return terraform, controllerruntime.Result{Requeue: true}, err
		}

		// This will create the "destroy" plan because deletion timestamp is set.
		traceLog.Info("Create a new plan to destroy")
		terraform, err = r.plan(ctx, terraform, tfInstance, runnerClient, revision)
		traceLog.Info("Check for error")
		if err != nil {
			traceLog.Error(err, "Error, requeue job")
			return terraform, controllerruntime.Result{Requeue: true}, err
		}

		traceLog.Info("Patch status of the Terraform resource")
		if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
			log.Error(err, "unable to update status after planing")
			return terraform, controllerruntime.Result{Requeue: true}, err
		}

		if thereIsNothingToDestroy(terraform) == false {
			traceLog.Info("Apply the destroy plan")
			terraform, err = r.apply(ctx, terraform, tfInstance, runnerClient, revision)
			traceLog.Info("Check for error")
			if err != nil {
				traceLog.Error(err, "Error, requeue job")
				return terraform, controllerruntime.Result{Requeue: true}, err
			}

			traceLog.Info("Patch status of the Terraform resource")
			if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
				log.Error(err, "unable to update status after applying")
				return terraform, controllerruntime.Result{Requeue: true}, err
			}

			traceLog.Info("Check for a nil error")
			if err == nil {
				log.Info("finalizing destroyResourcesOnDeletion: ok")
			}
		}

	}

	traceLog.Info("Check if we are writing output to secrets")
	outputSecretName := ""
	hasSpecifiedOutputSecret := terraform.Spec.WriteOutputsToSecret != nil && terraform.Spec.WriteOutputsToSecret.Name != ""
	if hasSpecifiedOutputSecret {
		traceLog.Info("Get the name of the output secret")
		outputSecretName = terraform.Spec.WriteOutputsToSecret.Name
	}

	traceLog.Info("Finalize the secrets")
	finalizeSecretsReply, err := runnerClient.FinalizeSecrets(ctx, &runner.FinalizeSecretsRequest{
		Namespace:                terraform.Namespace,
		Name:                     terraform.Name,
		Workspace:                terraform.WorkspaceName(),
		HasSpecifiedOutputSecret: hasSpecifiedOutputSecret,
		OutputSecretName:         outputSecretName,
	})
	traceLog.Info("Check for an error")
	if err != nil {
		traceLog.Info("Try getting a status from the error")
		if e, ok := status.FromError(err); ok {
			switch e.Code() {
			case codes.Internal:
				// transient error
				traceLog.Info("Internal error, transient, requeue")
				return terraform, controllerruntime.Result{Requeue: true}, err
			case codes.NotFound:
				// do nothing, fall through
				traceLog.Info("Not found, do nothing, fall through")
			}
		}
	}

	traceLog.Info("Check for an error")
	if err == nil {
		log.Info(fmt.Sprintf("finalizing secrets: %s", finalizeSecretsReply.Message))
	}

	// Record deleted status
	traceLog.Info("Record the deleted status")
	r.recordReadinessMetric(ctx, terraform)

	traceLog.Info("Get the Terraform resource")
	if err := r.Get(ctx, objectKey, &terraform); err != nil {
		traceLog.Error(err, "Hit an error, return")
		return terraform, controllerruntime.Result{}, err
	}

	// Remove our finalizer from the list and update it
	traceLog.Info("Remove the finalizer")
	controllerutil.RemoveFinalizer(&terraform, infrav1.TerraformFinalizer)
	traceLog.Info("Check for an error")
	if err := r.Update(ctx, &terraform); err != nil {
		traceLog.Error(err, "Hit an error, return")
		return terraform, controllerruntime.Result{}, err
	}

	// Remove the dependant finalizer from every dependency
	dependantFinalizer := infrav1.TFDependencyOfPrefix + terraform.GetName()
	for _, d := range terraform.Spec.DependsOn {
		if d.Namespace == "" {
			d.Namespace = terraform.GetNamespace()
		}
		dName := types.NamespacedName{
			Namespace: d.Namespace,
			Name:      d.Name,
		}
		var tf infrav1.Terraform
		err := r.Get(context.Background(), dName, &tf)
		if err != nil {
			return terraform, controllerruntime.Result{}, err
		}

		// add finalizer to the dependency
		if controllerutil.ContainsFinalizer(&tf, dependantFinalizer) {
			controllerutil.RemoveFinalizer(&tf, dependantFinalizer)
			if err := r.Update(context.Background(), &tf, client.FieldOwner(r.statusManager)); err != nil {
				return terraform, controllerruntime.Result{}, err
			}
		}
	}

	// Stop reconciliation as the object is being deleted
	traceLog.Info("Return success")
	return terraform, controllerruntime.Result{}, nil
}

func thereIsNothingToDestroy(terraform infrav1.Terraform) bool {
	// find condition with type "Plan"
	for _, c := range terraform.Status.Conditions {
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
