package controllers

import (
	"context"
	"fmt"

	"github.com/flux-iac/tofu-controller/api/planid"
	corev1 "k8s.io/api/core/v1"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *TerraformReconciler) shouldPlan(terraform *infrav1.Terraform) bool {
	// Please do not optimize this logic, as we'd like others to easily understand the logics behind this behaviour.
	if terraform.Spec.Force {
		return true
	}

	if terraform.Status.Plan.Pending == "" {
		return true
	} else if terraform.Status.Plan.Pending != "" {
		return false
	}
	return false
}

func (r *TerraformReconciler) plan(ctx context.Context, s *runnerSession, terraform *infrav1.Terraform) error {
	log := ctrl.LoggerFrom(ctx)

	log.Info("calling plan ...")

	infrav1.TerraformProgressing(terraform, "Terraform Planning")
	if err := r.progress(ctx, s, terraform, "unable to update status before Terraform planning"); err != nil {
		return err
	}

	const tfplanFilename = "tfplan"

	planRequest := &runner.PlanRequest{
		TfInstance:       s.tfInstance,
		Out:              tfplanFilename,
		Refresh:          true, // be careful, refresh requires to be true by default
		Targets:          terraform.Spec.Targets,
		SourceRefRootDir: s.tmpDir,
	}

	// if backend is disabled completely, there will be no plan output file (req.Out = "")
	if r.backendCompletelyDisable(terraform) {
		planRequest.Out = ""
	}

	// check if destroy is set to true or
	// the object is being deleted and DestroyResourcesOnDeletion is set to true
	if terraform.Spec.Destroy || (!terraform.DeletionTimestamp.IsZero() && terraform.Spec.DestroyResourcesOnDeletion) {
		log.Info("plan to destroy")
		planRequest.Destroy = true
	}

	if terraform.Spec.TFState != nil {
		if terraform.Spec.TFState.LockTimeout.Duration.String() != "" {
			log.Info("LockTimeout is set", "lockTimeout", terraform.Spec.TFState.LockTimeout)
			planRequest.LockTimeout = terraform.Spec.TFState.LockTimeout.Duration.String()
		}
	}

	planReply, err := s.client.Plan(ctx, planRequest)
	if err != nil {
		if lock, ok := stateLockFrom(err); ok {
			msg := fmt.Sprintf("Plan error: State locked with Lock Identifier %s", lock.Id)
			r.Eventf(terraform, nil, corev1.EventTypeWarning, infrav1.TFExecPlanFailedReason, infrav1.ActionPlan.String(), "%s", msg)
			infrav1.TerraformStateLocked(terraform, lock.Id, fmt.Sprintf("Terraform Locked with Lock Identifier: %s", lock.Id))
		} else {
			msg := fmt.Sprintf("Plan error: %s", err.Error())
			r.Eventf(terraform, nil, corev1.EventTypeWarning, infrav1.TFExecPlanFailedReason, infrav1.ActionPlan.String(), "%s", msg)
		}
		err = fmt.Errorf("error running Plan: %s", err)
		return notReady(terraform, s.revision, infrav1.TFExecPlanFailedReason, err)
	}

	drifted := planReply.Drifted
	log.Info("plan", "message", planReply.Message, "drifted", drifted)

	// currently the PlanCreated flag is only used here to determine if the destroy plan is empty
	if planRequest.Destroy && !planReply.PlanCreated {
		// A corner case
		// If the destroy plan is empty, we should not call apply
		infrav1.TerraformPlannedNoChanges(terraform, s.revision, "No objects need to be destroyed")
		return r.checkpoint(ctx, s, terraform)
	}

	if shouldProcessPostPlanningWebhooks(terraform) {
		log.Info("calling post planning webhooks ...")
		err = r.processPostPlanningWebhooks(ctx, s, terraform)
		if err != nil {
			log.Error(err, "failed during the process of post planning webhooks")
			infrav1.TerraformNotReady(
				terraform,
				s.revision,
				infrav1.PostPlanningWebhookFailedReason,
				err.Error(),
			)
			// The branch planner reads this outcome; the error returned is
			// the webhook's, whether or not the checkpoint lands.
			_ = r.checkpoint(ctx, s, terraform)
			return err
		}
	}

	saveTFPlanReply, err := s.client.SaveTFPlan(ctx, &runner.SaveTFPlanRequest{
		TfInstance:               s.tfInstance,
		BackendCompletelyDisable: r.backendCompletelyDisable(terraform),
		Name:                     terraform.Name,
		Namespace:                terraform.Namespace,
		Uuid:                     string(terraform.GetUID()),
		Revision:                 s.revision,
	})
	if err != nil {
		err = fmt.Errorf("error saving plan secret: %s", err)
		return notReady(terraform, s.revision, infrav1.TFExecPlanFailedReason, err)
	}
	log.Info("save tfplan", "message", saveTFPlanReply.Message)

	if drifted {
		forceOrAutoApply := r.forceOrAutoApply(terraform)

		// this is the manual mode, we fire the event to show how to apply the plan
		if !forceOrAutoApply {
			planId := planid.GetPlanID(s.revision)
			approveMessage := planid.GetApproveMessage(planId, "Plan generated")
			msg := fmt.Sprintf("Planned.\n%s", approveMessage)
			r.Eventf(terraform, nil, corev1.EventTypeNormal, infrav1.TFExecPlanSucceedReason, infrav1.ActionPlan.String(), "%s", msg)
		}
		infrav1.TerraformPlannedWithChanges(terraform, s.revision, forceOrAutoApply, "Plan generated")
		setPlanArtifactRef(terraform, saveTFPlanReply.SavedObjects, planid.GetPlanID(s.revision))
	} else {
		infrav1.TerraformPlannedNoChanges(terraform, s.revision, "Plan no changes")
		clearPlanArtifactRef(terraform)
	}

	return r.checkpoint(ctx, s, terraform)
}
