package controllers

import (
	"context"
	"encoding/json"

	"fmt"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *TerraformReconciler) shouldDetectDrift(terraform *infrav1.Terraform, revision string) bool {
	// Please do not optimize this logic, as we'd like others to easily understand the logics behind this behaviour.

	// return false when drift detection is disabled
	if terraform.Spec.DisableDriftDetection {
		return false
	}

	// not support when Destroy == true
	if terraform.Spec.Destroy {
		return false
	}

	// return true when approvePlan is disabled
	if terraform.Spec.ApprovePlan == infrav1.ApprovePlanDisableValue {
		return true
	}

	// new object
	if terraform.Status.LastAppliedRevision == "" &&
		terraform.Status.LastPlannedRevision == "" &&
		terraform.Status.LastAttemptedRevision == "" {
		return false
	}

	// spec change detected (i.e. vars), so dont detect drift.
	// we need a new plan!
	if terraform.Generation != terraform.Status.ObservedGeneration {
		return false
	}

	// thing worked normally, no change pending
	// then, we do drift detection
	if terraform.Status.LastAttemptedRevision == terraform.Status.LastAppliedRevision &&
		terraform.Status.LastAttemptedRevision == terraform.Status.LastPlannedRevision &&
		terraform.Status.LastAttemptedRevision == revision &&
		terraform.Status.Plan.Pending == "" {
		return true
	}

	// last time source changed with non-TF file, so we planned but no changes
	// this time, it needs drift detection
	if terraform.Status.LastAttemptedRevision == terraform.Status.LastPlannedRevision &&
		terraform.Status.LastAttemptedRevision == revision &&
		terraform.Status.Plan.Pending == "" {
		return true
	}

	return false
}

func (r *TerraformReconciler) detectDrift(ctx context.Context, s *runnerSession, terraform *infrav1.Terraform) error {
	log := ctrl.LoggerFrom(ctx)

	log.Info("calling detectDrift ...")

	const (
		driftFilename = "tfdrift"
	)

	planRequest := &runner.PlanRequest{
		TfInstance:       s.tfInstance,
		Out:              driftFilename,
		Refresh:          true,
		Targets:          terraform.Spec.Targets,
		SourceRefRootDir: s.tmpDir,
	}
	if r.backendCompletelyDisable(terraform) {
		planRequest.Out = ""
		planRequest.Refresh = true
	}

	planReply, err := s.client.Plan(ctx, planRequest)
	if err != nil {
		if lock, ok := stateLockFrom(err); ok {
			msg := fmt.Sprintf("Drift detection error: State locked with Lock Identifier %s", lock.Id)
			r.Eventf(terraform, nil, corev1.EventTypeWarning, infrav1.DriftDetectionFailedReason, infrav1.ActionDriftDetection.String(), "%s", msg)
			infrav1.TerraformStateLocked(terraform, lock.Id, fmt.Sprintf("Terraform Locked with Lock Identifier: %s", lock.Id))
		} else {
			msg := fmt.Sprintf("Drift detection error: %s", err.Error())
			r.Eventf(terraform, nil, corev1.EventTypeWarning, infrav1.DriftDetectionFailedReason, infrav1.ActionDriftDetection.String(), "%s", msg)
		}

		err = fmt.Errorf("error running Plan: %s", err)
		return notReady(terraform, s.revision, infrav1.DriftDetectionFailedReason, err)
	}
	drifted := planReply.Drifted
	log.Info("plan for drift", "message", planReply.Message, "drifted", planReply.Drifted)

	if drifted && !r.backendCompletelyDisable(terraform) {
		showReply, err := s.client.ShowPlanFile(ctx, &runner.ShowPlanFileRequest{
			TfInstance: s.tfInstance,
			Filename:   driftFilename,
		})
		if isPlanMessageSizeError(err) {
			log.Info("WARNING: JSON plan exceeds gRPC message-size limit; retaining exit-code-based drift detection", "error", err.Error())
		} else if err != nil {
			return notReady(terraform, s.revision, infrav1.DriftDetectionFailedReason, err)
		} else {
			var plan tfjson.Plan
			if err := json.Unmarshal(showReply.JsonOutput, &plan); err != nil {
				return notReady(terraform, s.revision, infrav1.DriftDetectionFailedReason, err)
			}
			if planHasOnlyDataReads(&plan) {
				drifted = false
				log.Info("ignoring data-source-only reads during drift detection")
			}
		}
	}

	if drifted {
		var rawOutput string
		if r.backendCompletelyDisable(terraform) {
			rawOutput = "not available"
		} else {
			showPlanFileRawReply, err := s.client.ShowPlanFileRaw(ctx, &runner.ShowPlanFileRawRequest{
				TfInstance: s.tfInstance,
				Filename:   driftFilename,
			})
			if err != nil {
				return notReady(terraform, s.revision, infrav1.DriftDetectionFailedReason, err)
			}
			rawOutput = showPlanFileRawReply.RawOutput
			log.Info("show plan", "output", showPlanFileRawReply.RawOutput)
		}

		// Clean up the message for Terraform v1.1.9.
		rawOutput = strings.Replace(rawOutput, "You can apply this plan to save these new output values to the Terraform\nstate, without changing any real infrastructure.", "", 1)

		msg := fmt.Sprintf("Drift detected.\n%s", rawOutput)
		r.Eventf(terraform, nil, corev1.EventTypeWarning, infrav1.DriftDetectedReason, infrav1.ActionDriftDetection.String(), "%s", msg)

		// If drift detected & we use the auto mode, then we continue
		infrav1.TerraformDriftDetected(terraform, s.revision, infrav1.DriftDetectedReason, rawOutput)
		return errDriftDetected
	}

	infrav1.TerraformNoDrift(terraform, s.revision, infrav1.NoDriftReason, "No drift")
	return nil
}
