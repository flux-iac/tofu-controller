package controllers

import (
	"context"
	"fmt"

	"github.com/flux-iac/tofu-controller/api/planid"
	eventv1 "github.com/fluxcd/pkg/apis/event/v1beta1"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *TerraformReconciler) shouldPlan(terraform infrav1.Terraform) bool {
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

func (r *TerraformReconciler) plan(ctx context.Context, terraform infrav1.Terraform, tfInstance string, runnerClient runner.RunnerClient, revision string, sourceRefRootDir string) (infrav1.Terraform, error) {

	log := ctrl.LoggerFrom(ctx)

	log.Info("calling plan ...")

	objectKey := types.NamespacedName{Namespace: terraform.Namespace, Name: terraform.Name}
	terraform = infrav1.TerraformProgressing(terraform, "Terraform Planning")
	if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
		log.Error(err, "unable to update status before Terraform planning")
		return terraform, err
	}

	const tfplanFilename = "tfplan"

	planRequest := &runner.PlanRequest{
		TfInstance:       tfInstance,
		Out:              tfplanFilename,
		Refresh:          true, // be careful, refresh requires to be true by default
		Targets:          terraform.Spec.Targets,
		SourceRefRootDir: sourceRefRootDir,
	}

	// if backend is disabled completely, there will be no plan output file (req.Out = "")
	if r.backendCompletelyDisable(terraform) {
		planRequest.Out = ""
	}

	// check if destroy is set to true or
	// the object is being deleted and DestroyResourcesOnDeletion is set to true
	if terraform.Spec.Destroy || (!terraform.ObjectMeta.DeletionTimestamp.IsZero() && terraform.Spec.DestroyResourcesOnDeletion) {
		log.Info("plan to destroy")
		planRequest.Destroy = true
	}

	if terraform.Spec.TFState != nil {
		if terraform.Spec.TFState.LockTimeout.Duration.String() != "" {
			log.Info(fmt.Sprintf("LockTimeout is set: %s", terraform.Spec.TFState.LockTimeout))
			planRequest.LockTimeout = terraform.Spec.TFState.LockTimeout.Duration.String()
		}
		if terraform.Spec.TFState.DisablePlanLock == true {
			planRequest.DisablePlanLock = true
		}
	}

	planReply, err := runnerClient.Plan(ctx, planRequest)
	if err != nil {

		eventSent := false
		if st, ok := status.FromError(err); ok {
			for _, detail := range st.Details() {
				if reply, ok := detail.(*runner.PlanReply); ok {
					msg := fmt.Sprintf("Plan error: State locked with Lock Identifier %s", reply.StateLockIdentifier)
					r.event(ctx, terraform, revision, eventv1.EventSeverityError, msg, nil)
					eventSent = true
					terraform = infrav1.TerraformStateLocked(terraform, reply.StateLockIdentifier, fmt.Sprintf("Terraform Locked with Lock Identifier: %s", reply.StateLockIdentifier))
				}
			}
		}

		if eventSent == false {
			msg := fmt.Sprintf("Plan error: %s", err.Error())
			r.event(ctx, terraform, revision, eventv1.EventSeverityError, msg, nil)
		}
		err = fmt.Errorf("error running Plan: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecPlanFailedReason,
			err.Error(),
		), err
	}

	drifted := planReply.Drifted
	log.Info(fmt.Sprintf("plan: %s, found drift: %v", planReply.Message, drifted))

	// currently the PlanCreated flag is only used here to determine if the destroy plan is empty
	if planRequest.Destroy && planReply.PlanCreated == false {
		// A corner case
		// If the destroy plan is empty, we should not call apply
		terraform = infrav1.TerraformPlannedNoChanges(terraform, revision, "No objects need to be destroyed")
		return terraform, nil
	}

	if shouldProcessPostPlanningWebhooks(terraform) {
		log.Info("calling post planning webhooks ...")
		terraform, err = r.processPostPlanningWebhooks(ctx, terraform, runnerClient, revision, tfInstance)
		if err != nil {
			log.Error(err, "failed during the process of post planning webhooks")
			return infrav1.TerraformNotReady(
				terraform,
				revision,
				infrav1.PostPlanningWebhookFailedReason,
				err.Error(),
			), err
		}
	}

	saveTFPlanReply, err := runnerClient.SaveTFPlan(ctx, &runner.SaveTFPlanRequest{
		TfInstance:               tfInstance,
		BackendCompletelyDisable: r.backendCompletelyDisable(terraform),
		Name:                     terraform.Name,
		Namespace:                terraform.Namespace,
		Uuid:                     string(terraform.GetUID()),
		Revision:                 revision,
	})
	if err != nil {
		err = fmt.Errorf("error saving plan secret: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecPlanFailedReason,
			err.Error(),
		), err
	}
	log.Info(fmt.Sprintf("save tfplan: %s", saveTFPlanReply.Message))

	if drifted {
		forceOrAutoApply := r.forceOrAutoApply(terraform)

		// this is the manual mode, we fire the event to show how to apply the plan
		if forceOrAutoApply == false {
			planId := planid.GetPlanID(revision)
			approveMessage := planid.GetApproveMessage(planId, "Plan generated")
			msg := fmt.Sprintf("Planned.\n%s", approveMessage)
			r.event(ctx, terraform, revision, eventv1.EventSeverityInfo, msg, nil)
		}
		terraform = infrav1.TerraformPlannedWithChanges(terraform, revision, forceOrAutoApply, "Plan generated")
	} else {
		terraform = infrav1.TerraformPlannedNoChanges(terraform, revision, "Plan no changes")
	}

	return terraform, nil
}
