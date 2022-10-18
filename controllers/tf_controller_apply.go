package controllers

import (
	"context"
	"fmt"
	"github.com/fluxcd/pkg/runtime/logger"
	"strings"

	"github.com/fluxcd/pkg/runtime/events"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha1"
	"github.com/weaveworks/tf-controller/runner"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *TerraformReconciler) forceOrAutoApply(ctx context.Context, terraform infrav1.Terraform) bool {
	log := ctrl.LoggerFrom(ctx)
	traceLog := log.V(logger.TraceLevel).WithValues("function", "TerraformReconciler.forceOrAutoApply")

	traceLog.Info("Checking if force is set or auto apply is enabled", "force", terraform.Spec.Force, "autoApply", terraform.Spec.ApprovePlan)
	result := terraform.Spec.Force || terraform.Spec.ApprovePlan == infrav1.ApprovePlanAutoValue
	traceLog.Info("Result", "result", result)
	return result
}

func (r *TerraformReconciler) shouldApply(ctx context.Context, terraform infrav1.Terraform) bool {
	log := ctrl.LoggerFrom(ctx)
	traceLog := log.V(logger.TraceLevel).WithValues("function", "TerraformReconciler.shouldApply")

	traceLog.Info("Entering shouldApply",
		"approvePlan", terraform.Spec.ApprovePlan,
		"force", terraform.Spec.Force,
		"planPending", terraform.Status.Plan.Pending)

	// Please do not optimize this logic, as we'd like others to easily understand the logics behind this behaviour
	traceLog.Info("Checking if force is set", "force", terraform.Spec.Force)
	if terraform.Spec.Force {
		traceLog.Info("Force is set, returning true")
		return true
	}

	traceLog.Info("Checking current state of approvePlan")
	if terraform.Spec.ApprovePlan == "" {
		traceLog.Info("ApprovePlan is empty, returning false - should not apply")
		return false
	} else if terraform.Spec.ApprovePlan == infrav1.ApprovePlanAutoValue && terraform.Status.Plan.Pending != "" {
		traceLog.Info("ApprovePlan is auto and plan is pending, returning true - should apply", "plan Pending", terraform.Status.Plan.Pending)
		return true
	} else if terraform.Spec.ApprovePlan == terraform.Status.Plan.Pending {
		traceLog.Info("ApprovePlan is set and matches plan pending, returning true - should apply", "plan Pending", terraform.Status.Plan.Pending)
		return true
	} else if strings.HasPrefix(terraform.Status.Plan.Pending, terraform.Spec.ApprovePlan) {
		traceLog.Info("ApprovePlan is set and matches plan pending prefix, returning true - should apply",
			"plan Pending", terraform.Status.Plan.Pending, "approvePlan", terraform.Spec.ApprovePlan)
		return true
	}

	traceLog.Info("Otherwise, returning false - should not apply")
	return false
}

func (r *TerraformReconciler) apply(ctx context.Context, terraform infrav1.Terraform, tfInstance string, runnerClient runner.RunnerClient, revision string) (infrav1.Terraform, error) {

	const (
		TFPlanName = "tfplan"
	)

	log := ctrl.LoggerFrom(ctx)
	objectKey := types.NamespacedName{Namespace: terraform.Namespace, Name: terraform.Name}

	terraform = infrav1.TerraformProgressing(terraform, "Applying")
	if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
		log.Error(err, "unable to update status before Terraform applying")
		return terraform, err
	}

	loadTFPlanReply, err := runnerClient.LoadTFPlan(ctx, &runner.LoadTFPlanRequest{
		TfInstance:               tfInstance,
		Name:                     terraform.Name,
		Namespace:                terraform.Namespace,
		BackendCompletelyDisable: r.backendCompletelyDisable(terraform),
		PendingPlan:              terraform.Status.Plan.Pending,
	})
	if err != nil {
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.TFExecApplyFailedReason,
			err.Error(),
		), err
	}

	log.Info(fmt.Sprintf("load tf plan: %s", loadTFPlanReply.Message))

	terraform = infrav1.TerraformApplying(terraform, revision, "Apply started")
	if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
		log.Error(err, "error recording apply status: %s", err)
		return terraform, err
	}

	applyRequest := &runner.ApplyRequest{
		TfInstance:         tfInstance,
		RefreshBeforeApply: terraform.Spec.RefreshBeforeApply,
		Targets:            terraform.Spec.Targets,
	}
	if r.backendCompletelyDisable(terraform) {
		// do nothing
	} else {
		applyRequest.DirOrPlan = TFPlanName
	}

	var isDestroyApplied bool

	var inventoryEntries []infrav1.ResourceRef

	// this a special case, when backend is completely disabled.
	// we need to use "destroy" command instead of apply
	if r.backendCompletelyDisable(terraform) && terraform.Spec.Destroy == true {
		destroyReply, err := runnerClient.Destroy(ctx, &runner.DestroyRequest{
			TfInstance: tfInstance,
			Targets:    terraform.Spec.Targets,
		})
		log.Info(fmt.Sprintf("destroy: %s", destroyReply.Message))

		eventSent := false
		if err != nil {
			if st, ok := status.FromError(err); ok {
				for _, detail := range st.Details() {
					if reply, ok := detail.(*runner.DestroyReply); ok {
						msg := fmt.Sprintf("Destroy error: State locked with Lock Identifier %s", reply.StateLockIdentifier)
						r.event(ctx, terraform, revision, events.EventSeverityError, msg, nil)
						eventSent = true
						terraform = infrav1.TerraformStateLocked(terraform, reply.StateLockIdentifier, fmt.Sprintf("Terraform Locked with Lock Identifier: %s", reply.StateLockIdentifier))
					}
				}
			}

			if eventSent == false {
				msg := fmt.Sprintf("Destroy error: %s", err.Error())
				r.event(ctx, terraform, revision, events.EventSeverityError, msg, nil)
			}

			err = fmt.Errorf("error running Destroy: %s", err)
			return infrav1.TerraformAppliedFailResetPlanAndNotReady(
				terraform,
				revision,
				infrav1.TFExecApplyFailedReason,
				err.Error(),
			), err
		}
		isDestroyApplied = true
	} else {
		eventSent := false
		applyReply, err := runnerClient.Apply(ctx, applyRequest)
		if err != nil {
			if st, ok := status.FromError(err); ok {
				for _, detail := range st.Details() {
					if reply, ok := detail.(*runner.ApplyReply); ok {
						msg := fmt.Sprintf("Apply error: State locked with Lock Identifier %s", reply.StateLockIdentifier)
						r.event(ctx, terraform, revision, events.EventSeverityError, msg, nil)
						eventSent = true
						terraform = infrav1.TerraformStateLocked(terraform, reply.StateLockIdentifier, fmt.Sprintf("Terraform Locked with Lock Identifier: %s", reply.StateLockIdentifier))
					}
				}
			}

			if eventSent == false {
				msg := fmt.Sprintf("Apply error: %s", err.Error())
				r.event(ctx, terraform, revision, events.EventSeverityError, msg, nil)
			}

			err = fmt.Errorf("error running Apply: %s", err)
			return infrav1.TerraformAppliedFailResetPlanAndNotReady(
				terraform,
				revision,
				infrav1.TFExecApplyFailedReason,
				err.Error(),
			), err
		}
		log.Info(fmt.Sprintf("apply: %s", applyReply.Message))

		isDestroyApplied = terraform.Status.Plan.IsDestroyPlan

		// if apply was successful, we need to update the inventory, but not if we are destroying
		if terraform.Spec.EnableInventory && isDestroyApplied == false {
			getInventoryRequest := &runner.GetInventoryRequest{TfInstance: tfInstance}
			getInventoryReply, err := runnerClient.GetInventory(ctx, getInventoryRequest)
			if err != nil {
				err = fmt.Errorf("error getting inventory after Apply: %s", err)
				return infrav1.TerraformAppliedFailResetPlanAndNotReady(
					terraform,
					revision,
					infrav1.TFExecApplyFailedReason,
					err.Error(),
				), err
			}
			for _, iv := range getInventoryReply.Inventories {
				inventoryEntries = append(inventoryEntries, infrav1.ResourceRef{
					Name:       iv.GetName(),
					Type:       iv.GetType(),
					Identifier: iv.GetIdentifier(),
				})
			}
			log.Info(fmt.Sprintf("got inventory - entries count: %d", len(inventoryEntries)))
		} else {
			log.Info("inventory is disabled by default")
		}
	}

	if isDestroyApplied {
		msg := fmt.Sprintf("Destroy applied successfully")
		r.event(ctx, terraform, revision, events.EventSeverityInfo, msg, nil)
		terraform = infrav1.TerraformApplied(terraform, revision, "Destroy applied successfully", isDestroyApplied, inventoryEntries)
	} else {
		msg := fmt.Sprintf("Applied successfully")
		r.event(ctx, terraform, revision, events.EventSeverityInfo, msg, nil)
		terraform = infrav1.TerraformApplied(terraform, revision, "Applied successfully", isDestroyApplied, inventoryEntries)
	}

	return terraform, nil
}
