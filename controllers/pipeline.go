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
	"errors"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// reconcileNormal runs one pass of the pipeline against an open runner
// session: prepare the working directory, then detect drift, plan, apply,
// write outputs, run health checks and record the inventory. Steps mutate
// obj; status is written by progress checkpoints, durable checkpoints and the
// caller's final patch.
func (r *TerraformReconciler) reconcileNormal(ctx context.Context, s *runnerSession, obj *infrav1.Terraform, src sourcev1.Source) error {
	// Upload the source and initialise Terraform in a working directory. The
	// directory is removed when the pass ends, even if setup fails after
	// creating it.
	defer s.cleanupDir(ctx)
	if err := r.setup(ctx, s, obj, src); err != nil {
		return err
	}

	// A break-the-glass request replaces the rest of the pass.
	if breakGlassRequested(obj) {
		return r.breakGlass(ctx, s, obj)
	}

	// Compare the applied state with the real resources.
	if r.shouldDetectDrift(obj, s.revision) {
		err := r.detectDrift(ctx, s, obj)
		switch {
		case err == nil:
			// No drift: refresh outputs only if the outputs secret may be out of date.
			return r.refreshOutputsIfDrifted(ctx, s, obj)
		case !errors.Is(err, errDriftDetected):
			return err
		case !r.forceOrAutoApply(obj):
			// Drift detected, and fixing it needs a manual approval.
			return err
		}
		// Drift detected with force or auto-apply: continue to plan and apply.
	}

	// approvePlan "disable" stops here: only drift detection runs.
	if obj.Spec.ApprovePlan == infrav1.ApprovePlanDisableValue {
		return nil
	}

	if r.shouldPlan(obj) {
		if err := r.plan(ctx, s, obj); err != nil {
			return err
		}
	}

	if r.shouldApply(obj) {
		// A manual approval is re-checked against a fresh plan, so a plan
		// that changed since it was approved is not applied.
		if r.needsApprovalRecheck(obj) {
			if err := r.plan(ctx, s, obj); err != nil {
				return err
			}
			if !r.shouldApply(obj) {
				ctrl.LoggerFrom(ctx).Info("plan changed while waiting for manual approval, waiting for new approval",
					"approvedPlan", obj.Spec.ApprovePlan, "pendingPlan", obj.Status.Plan.Pending)
				return nil
			}
		}
		if err := r.apply(ctx, s, obj); err != nil {
			return err
		}
	}

	if err := r.processOutputs(ctx, s, obj); err != nil {
		return err
	}

	if r.shouldDoHealthChecks(obj) {
		if err := r.doHealthChecks(ctx, s, obj); err != nil {
			return err
		}
	}

	r.syncInventory(ctx, s, obj)

	// Mark Ready True when the pass has settled: nothing is still in
	// progress and no plan waits for approval.
	markReadyIfSettled(obj)
	return nil
}

// needsApprovalRecheck reports whether a manually approved plan must be
// re-planned before it is applied, to make sure the approval still matches.
func (r *TerraformReconciler) needsApprovalRecheck(obj *infrav1.Terraform) bool {
	return !r.forceOrAutoApply(obj) &&
		obj.Spec.ApprovePlan != "" &&
		obj.Spec.ApprovePlan != infrav1.ApprovePlanAutoValue
}
