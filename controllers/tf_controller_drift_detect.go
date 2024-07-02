package controllers

import (
	"context"
	"fmt"
	"strings"

	eventv1 "github.com/fluxcd/pkg/apis/event/v1beta1"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"
	"google.golang.org/grpc/status"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *TerraformReconciler) shouldDetectDrift(terraform infrav1.Terraform, revision string) bool {
	// Please do not optimize this logic, as we'd like others to easily understand the logics behind this behaviour.

	// return false when drift detection is disabled
	if terraform.Spec.DisableDriftDetection == true {
		return false
	}

	// not support when Destroy == true
	if terraform.Spec.Destroy == true {
		return false
	}

	if terraform.Spec.ApprovePlan == infrav1.ApprovePlanDisableValue {
		return true
	}

	// new object
	if terraform.Status.LastAppliedRevision == "" &&
		terraform.Status.LastPlannedRevision == "" &&
		terraform.Status.LastAttemptedRevision == "" {
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

func (r *TerraformReconciler) detectDrift(ctx context.Context, terraform infrav1.Terraform, tfInstance string, runnerClient runner.RunnerClient, revision string, sourceRefRootDir string) (infrav1.Terraform, error) {

	log := ctrl.LoggerFrom(ctx)

	log.Info("calling detectDrift ...")

	const (
		driftFilename = "tfdrift"
	)

	planRequest := &runner.PlanRequest{
		TfInstance:       tfInstance,
		Out:              driftFilename,
		Refresh:          true,
		Targets:          terraform.Spec.Targets,
		SourceRefRootDir: sourceRefRootDir,
	}
	if r.backendCompletelyDisable(terraform) {
		planRequest.Out = ""
		planRequest.Refresh = true
	}

	eventSent := false
	planReply, err := runnerClient.Plan(ctx, planRequest)
	if err != nil {
		if st, ok := status.FromError(err); ok {
			for _, detail := range st.Details() {
				if reply, ok := detail.(*runner.PlanReply); ok {
					msg := fmt.Sprintf("Drift detection error: State locked with Lock Identifier %s", reply.StateLockIdentifier)
					r.event(ctx, terraform, revision, eventv1.EventSeverityError, msg, nil)
					eventSent = true
					terraform = infrav1.TerraformStateLocked(terraform, reply.StateLockIdentifier, fmt.Sprintf("Terraform Locked with Lock Identifier: %s", reply.StateLockIdentifier))
				}
			}
		}

		if eventSent == false {
			msg := fmt.Sprintf("Drift detection error: %s", err.Error())
			r.event(ctx, terraform, revision, eventv1.EventSeverityError, msg, nil)
		}

		err = fmt.Errorf("error running Plan: %s", err)
		return infrav1.TerraformNotReady(
			terraform,
			revision,
			infrav1.DriftDetectionFailedReason,
			err.Error(),
		), err
	}
	drifted := planReply.Drifted
	log.Info(fmt.Sprintf("plan for drift: %s found drift: %v", planReply.Message, planReply.Drifted))

	if drifted {
		var rawOutput string
		if r.backendCompletelyDisable(terraform) {
			rawOutput = "not available"
		} else {
			showPlanFileRawReply, err := runnerClient.ShowPlanFileRaw(ctx, &runner.ShowPlanFileRawRequest{
				TfInstance: tfInstance,
				Filename:   driftFilename,
			})
			if err != nil {
				return infrav1.TerraformNotReady(
					terraform,
					revision,
					infrav1.DriftDetectionFailedReason,
					err.Error(),
				), err
			}
			rawOutput = showPlanFileRawReply.RawOutput
			log.Info(fmt.Sprintf("show plan: %s", showPlanFileRawReply.RawOutput))
		}

		// Clean up the message for Terraform v1.1.9.
		rawOutput = strings.Replace(rawOutput, "You can apply this plan to save these new output values to the Terraform\nstate, without changing any real infrastructure.", "", 1)

		msg := fmt.Sprintf("Drift detected.\n%s", rawOutput)
		r.event(ctx, terraform, revision, eventv1.EventSeverityError, msg, nil)

		// If drift detected & we use the auto mode, then we continue
		terraform = infrav1.TerraformDriftDetected(terraform, revision, infrav1.DriftDetectedReason, rawOutput)
		return terraform, fmt.Errorf(infrav1.DriftDetectedReason)
	}

	terraform = infrav1.TerraformNoDrift(terraform, revision, infrav1.NoDriftReason, "No drift")
	return terraform, nil
}
