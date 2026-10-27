package controllers

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *TerraformReconciler) forceOrAutoApply(terraform *infrav1.Terraform) bool {
	return terraform.Spec.Force || terraform.Spec.ApprovePlan == infrav1.ApprovePlanAutoValue
}

func (r *TerraformReconciler) shouldApply(terraform *infrav1.Terraform) bool {
	// Please do not optimize this logic, as we'd like others to easily understand the logics behind this behaviour.
	if terraform.Spec.Force {
		return true
	}

	if terraform.Spec.PlanOnly {
		return false
	}

	if terraform.Spec.ApprovePlan == "" {
		return false
	} else if terraform.Spec.ApprovePlan == infrav1.ApprovePlanAutoValue && terraform.Status.Plan.Pending != "" {
		return true
	} else if terraform.Spec.ApprovePlan == terraform.Status.Plan.Pending {
		return true
	} else if strings.HasPrefix(terraform.Status.Plan.Pending, terraform.Spec.ApprovePlan) {
		return true
	}
	return false
}

func (r *TerraformReconciler) apply(ctx context.Context, s *runnerSession, terraform *infrav1.Terraform) error {
	const (
		TFPlanName = "tfplan"
	)

	log := ctrl.LoggerFrom(ctx)

	infrav1.TerraformProgressing(terraform, "Applying")
	if err := r.progress(ctx, s, terraform, "unable to update status before Terraform applying"); err != nil {
		return err
	}

	loadTFPlanReply, err := s.client.LoadTFPlan(ctx, &runner.LoadTFPlanRequest{
		TfInstance:               s.tfInstance,
		Name:                     terraform.Name,
		Namespace:                terraform.Namespace,
		BackendCompletelyDisable: r.backendCompletelyDisable(terraform),
		PendingPlan:              terraform.Status.Plan.Pending,
	})
	if err != nil {
		// replan if errors occur
		terraform.Status.Plan.Pending = ""
		terraform.Status.LastPlannedRevision = ""
		return notReady(terraform, s.revision, infrav1.TFExecApplyFailedReason, err)
	}

	log.Info("load tf plan", "message", loadTFPlanReply.Message)

	infrav1.TerraformApplying(terraform, s.revision, "Apply started")
	if err := r.progress(ctx, s, terraform, "error recording apply status"); err != nil {
		return err
	}

	applyRequest := &runner.ApplyRequest{
		TfInstance:         s.tfInstance,
		Parallelism:        terraform.Spec.Parallelism,
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
	if r.backendCompletelyDisable(terraform) && terraform.Spec.Destroy {
		destroyReply, err := s.client.Destroy(ctx, &runner.DestroyRequest{
			TfInstance: s.tfInstance,
			Targets:    terraform.Spec.Targets,
		})

		if err != nil {
			if lock, ok := stateLockFrom(err); ok {
				msg := fmt.Sprintf("Destroy error: State locked with Lock Identifier %s", lock.Id)
				r.Eventf(terraform, nil, corev1.EventTypeWarning, infrav1.TFExecDestroyFailedReason, infrav1.ActionDestroy.String(), "%s", msg)
				infrav1.TerraformStateLocked(terraform, lock.Id, fmt.Sprintf("Terraform Locked with Lock Identifier: %s", lock.Id))
			} else {
				msg := fmt.Sprintf("Destroy error: %s", err.Error())
				r.Eventf(terraform, nil, corev1.EventTypeWarning, infrav1.TFExecDestroyFailedReason, infrav1.ActionDestroy.String(), "%s", msg)
			}

			err = fmt.Errorf("error running Destroy: %s", err)
			infrav1.TerraformAppliedFailResetPlanAndNotReady(
				terraform,
				s.revision,
				infrav1.TFExecApplyFailedReason,
				err.Error(),
			)
			_ = r.checkpoint(ctx, s, terraform) // the apply error is returned either way
			return err
		}
		log.Info("destroy", "message", destroyReply.Message)
		isDestroyApplied = true
	} else {
		applyReply, err := s.client.Apply(ctx, applyRequest)
		if err != nil {
			if lock, ok := stateLockFrom(err); ok {
				msg := fmt.Sprintf("Apply error: State locked with Lock Identifier %s", lock.Id)
				r.Eventf(terraform, nil, corev1.EventTypeWarning, infrav1.TFExecApplyFailedReason, infrav1.ActionApply.String(), "%s", msg)
				infrav1.TerraformStateLocked(terraform, lock.Id, fmt.Sprintf("Terraform Locked with Lock Identifier: %s", lock.Id))
			} else {
				msg := fmt.Sprintf("Apply error: %s", err.Error())
				r.Eventf(terraform, nil, corev1.EventTypeWarning, infrav1.TFExecApplyFailedReason, infrav1.ActionApply.String(), "%s", msg)
			}

			err = fmt.Errorf("error running Apply: %s", err)
			infrav1.TerraformAppliedFailResetPlanAndNotReady(
				terraform,
				s.revision,
				infrav1.TFExecApplyFailedReason,
				err.Error(),
			)
			_ = r.checkpoint(ctx, s, terraform) // the apply error is returned either way
			return err
		}
		log.Info("apply", "message", applyReply.Message)

		isDestroyApplied = terraform.Status.Plan.IsDestroyPlan

		// if apply was successful, we need to update the inventory, but not if we are destroying
		if terraform.Spec.EnableInventory && !isDestroyApplied {
			getInventoryRequest := &runner.GetInventoryRequest{TfInstance: s.tfInstance}
			getInventoryReply, err := s.client.GetInventory(ctx, getInventoryRequest)
			if err != nil {
				err = fmt.Errorf("error getting inventory after Apply: %s", err)
				infrav1.TerraformAppliedFailResetPlanAndNotReady(
					terraform,
					s.revision,
					infrav1.TFExecApplyFailedReason,
					err.Error(),
				)
				_ = r.checkpoint(ctx, s, terraform) // the apply error is returned either way
				return err
			}

			// TODO add resource location to inventory
			for _, iv := range getInventoryReply.Inventories {
				inventoryEntries = append(inventoryEntries, infrav1.ResourceRef{
					Name:       iv.GetName(),
					Type:       iv.GetType(),
					Identifier: iv.GetIdentifier(),
				})
			}
			log.Info("got inventory", "entries", len(inventoryEntries))
		} else if !terraform.Spec.EnableInventory {
			log.Info("inventory is disabled")
			terraform.Status.Inventory = nil
		}
	}

	var msg string
	if isDestroyApplied {
		msg = "Destroy applied successfully"
		r.Eventf(terraform, nil, corev1.EventTypeNormal, infrav1.TFExecDestroySucceedReason, infrav1.ActionDestroy.String(), "%s", msg)
	} else {
		msg = "Applied successfully"
		r.Eventf(terraform, nil, corev1.EventTypeNormal, infrav1.TFExecApplySucceedReason, infrav1.ActionApply.String(), "%s", msg)
	}

	infrav1.TerraformApplied(terraform, s.revision, msg, isDestroyApplied, inventoryEntries)

	return r.checkpoint(ctx, s, terraform)
}
