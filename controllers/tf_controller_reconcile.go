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
	"context"
	"fmt"
	"strings"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"
	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *TerraformReconciler) reconcile(ctx context.Context, runnerClient runner.RunnerClient, terraform infrav1.Terraform, sourceObj sourcev1.Source, reconciliationLoopID string) (*infrav1.Terraform, error) {
	log := ctrl.LoggerFrom(ctx)
	revision := sourceObj.GetArtifact().Revision
	objectKey := types.NamespacedName{Namespace: terraform.Namespace, Name: terraform.Name}

	var (
		tfInstance string
		tmpDir     string
		err        error

		lastKnownAction string
	)
	log.Info("setting up terraform")
	terraform, tfInstance, tmpDir, err = r.setupTerraform(ctx, runnerClient, terraform, sourceObj, revision, objectKey, reconciliationLoopID)

	lastKnownAction = "Setup"

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
		log.Error(err, "error in terraform setup")
		return &terraform, err
	}

	if r.AllowBreakTheGlass {
		// spec.breakTheGlass || annotation
		breakTheGlass := terraform.Spec.BreakTheGlass
		if terraform.Annotations != nil {
			_, exist := terraform.Annotations[infrav1.BreakTheGlassAnnotation]
			if exist {
				breakTheGlass = true
			}
		}

		if breakTheGlass {
			// ok, change status to "break the glass"
			terraform = infrav1.TerraformProgressing(terraform, "Breaking the glass ...")
			if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
				log.Error(err, "unable to update status after drift detection")
				return &terraform, err
			}

			// create /tmp/.break-glass file
			_, err := runnerClient.StartBreakTheGlassSession(ctx, &runner.BreakTheGlassRequest{})
			if err != nil {
				log.Error(err, "error starting break the glass session")
				return &terraform, err
			}

			done := false
			for !done {
				// check /tmp/.break-glass file exists
				reply, err := runnerClient.HasBreakTheGlassSessionDone(ctx, &runner.BreakTheGlassRequest{})
				if err != nil {
					return nil, err
				}
				if reply.Success {
					done = true
				}

				time.Sleep(10 * time.Second)
			}

			// set status back to "Initializing"
			terraform = infrav1.TerraformProgressing(terraform, "Initializing")
			if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
				log.Error(err, "unable to update status after drift detection")
				return &terraform, err
			}

			return &terraform, fmt.Errorf("break the glass session has ended at %v", time.Now().Format(time.RFC3339))
		}
	} else {
		// spec.breakTheGlass || annotation
		breakTheGlass := terraform.Spec.BreakTheGlass
		if terraform.Annotations != nil {
			_, exist := terraform.Annotations[infrav1.BreakTheGlassAnnotation]
			if exist {
				breakTheGlass = true
			}
		}

		if breakTheGlass {
			terraform = infrav1.TerraformProgressing(terraform, "Breaking the glass is not allowed")
			if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
				log.Error(err, "unable to update status after drift detection")
				return &terraform, err
			}

			log.Info("break the glass is not allowed")
			return &terraform, fmt.Errorf("break the glass is not allowed")
		}
	}

	if r.shouldDetectDrift(terraform, revision) {
		var driftDetectionErr error // declared here to avoid shadowing on terraform variable
		terraform, driftDetectionErr = r.detectDrift(ctx, terraform, tfInstance, runnerClient, revision, tmpDir)

		// immediately return if no drift - reconciliation will retry normally
		if driftDetectionErr == nil {
			// reconcile outputs only when outputs are missing
			if outputsDrifted, err := r.outputsMayBeDrifted(ctx, terraform); outputsDrifted == true && err == nil {
				terraform, err = r.processOutputs(ctx, runnerClient, terraform, tfInstance, revision)
				if err != nil {
					log.Error(err, "error processing outputs")
					return &terraform, err
				}
			} else if err != nil {
				log.Error(err, "error checking for output drift")
				return &terraform, err
			}

			return &terraform, nil
		}

		// immediately return if err is not about drift
		if driftDetectionErr.Error() != infrav1.DriftDetectedReason {
			log.Error(driftDetectionErr, "detected non drift error")
			return &terraform, driftDetectionErr
		}

		// immediately return if drift is detected, but it's not "force" or "auto"
		if driftDetectionErr.Error() == infrav1.DriftDetectedReason && !r.forceOrAutoApply(terraform) {
			log.Error(driftDetectionErr, "will not force / auto apply detected drift")
			return &terraform, driftDetectionErr
		}

		// ok Drift Detected, patch and continue
		if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
			log.Error(err, "unable to update status after drift detection")
			return &terraform, err
		}

		lastKnownAction = "Drift Detection"
	}

	// return early if we're in drift-detection-only mode
	if terraform.Spec.ApprovePlan == infrav1.ApprovePlanDisableValue {
		log.Info("approve plan disabled")
		return &terraform, nil
	}

	// if we should plan this Terraform CR, do so
	if r.shouldPlan(terraform) {
		terraform, err = r.plan(ctx, terraform, tfInstance, runnerClient, revision, tmpDir)
		if err != nil {
			log.Error(err, "error planning")
			return &terraform, err
		}

		if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
			log.Error(err, "unable to update status after planing")
			return &terraform, err
		}

		lastKnownAction = "Planned"
	}

	// if we should apply the generated plan, do so
	if r.shouldApply(terraform) {
		terraform, err = r.apply(ctx, terraform, tfInstance, runnerClient, revision)
		if err != nil {
			log.Error(err, "error applying")
			return &terraform, err
		}

		if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
			log.Error(err, "unable to update status after applying")
			return &terraform, err
		}

		lastKnownAction = "Applied"
	} else {
		log.Info("should apply == false")
	}

	terraform, err = r.processOutputs(ctx, runnerClient, terraform, tfInstance, revision)
	if err != nil {
		log.Error(err, "error process outputs")
		return &terraform, err
	}
	lastKnownAction = "Outputs Processed"

	if r.shouldDoHealthChecks(terraform) {

		terraform, err = r.doHealthChecks(ctx, terraform, revision, runnerClient)
		if err != nil {
			log.Error(err, "error with health check")
			return &terraform, err
		}

		if err := r.patchStatus(ctx, objectKey, terraform.Status); err != nil {
			log.Error(err, "unable to update status after doing health checks")
			return &terraform, err
		}

		lastKnownAction = "Health Checked"
	}

	// Update the inventory of the Terraform CR, in case it went missing.
	if terraform.Spec.EnableInventory && terraform.Status.Inventory == nil {
		var inventoryEntries []infrav1.ResourceRef

		getInventoryRequest := &runner.GetInventoryRequest{TfInstance: tfInstance}
		getInventoryReply, err := runnerClient.GetInventory(ctx, getInventoryRequest)
		if err != nil {
			log.Error(err, "error getting inventory")
		}

		for _, iv := range getInventoryReply.Inventories {
			inventoryEntries = append(inventoryEntries, infrav1.ResourceRef{
				Name:       iv.GetName(),
				Type:       iv.GetType(),
				Identifier: iv.GetIdentifier(),
			})
		}
		log.Info(fmt.Sprintf("got inventory - entries count: %d", len(inventoryEntries)))

		if len(inventoryEntries) > 0 {
			terraform.Status.Inventory = &infrav1.ResourceInventory{
				Entries: inventoryEntries,
			}
		}
	} else if terraform.Spec.EnableInventory == false {
		log.Info("inventory is disabled")
		terraform.Status.Inventory = nil
	}

	var (
		readyCondition      *metav1.Condition
		readyConditionIndex int
	)
	for i, condition := range terraform.Status.Conditions {
		if condition.Type == meta.ReadyCondition {
			readyCondition = &condition
			readyConditionIndex = i
			break
		}
	}

	if readyCondition != nil && readyCondition.Status == metav1.ConditionUnknown {
		log.Info("Last known action was: " + lastKnownAction)
		cond := terraform.Status.Conditions[readyConditionIndex]
		if cond.Reason == infrav1.PlannedWithChangesReason && strings.HasPrefix(cond.Message, "Plan generated") {
			// do nothing
		} else if cond.Reason != meta.ProgressingReason {
			terraform.Status.Conditions[readyConditionIndex].Status = metav1.ConditionTrue
		}
	}

	return &terraform, nil
}
