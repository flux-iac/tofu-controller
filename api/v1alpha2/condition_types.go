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

package v1alpha2

import "github.com/fluxcd/pkg/apis/meta"

// OwnedConditions are the Condition Types that the Terraform Resource owns
var OwnedConditions = []string{
	meta.ReconcilingCondition,
	meta.ReadyCondition,
	meta.StalledCondition,
}

// These constants are the Condition Types that the Terraform Resource works with
const (
	ConditionTypeApply       = "Apply"
	ConditionTypeHealthCheck = "HealthCheck"
	ConditionTypeOutput      = "Output"
	ConditionTypePlan        = "Plan"
	ConditionTypeStateLocked = "StateLocked"
)

const (
	// AccessDeniedReason represents the fact that access to
	// a resource was denied by an ACL assertion.
	AccessDeniedReason = "AccessDenied"

	// ArtifactFailedReason represents the fact that the artifact download
	// for the Teraform failed.
	ArtifactFailedReason = "ArtifactFailed"

	// RetryLimitReachedReason represents the fact that the Terraform
	// reached the maximum number of retries.
	RetryLimitReachedReason = "RetryLimitReached"

	// DeletionBlockedByDependantsReason represents the fact that the
	// Terraform resource could not be deleted because there are
	// still resources depending on it.
	DeletionBlockedByDependants = "DeletionBlockedByDependantsReason"

	// DependencyNotReadyReason represents the fact that
	// one of the dependencies is not ready.
	DependencyNotReadyReason = "DependencyNotReady"

	// DriftDetectedReason represents the fact that drift was
	// detected during Terraform reconciliation.
	DriftDetectedReason = "DriftDetected"

	// DriftDetectionFailedReason represents the fact that
	// drift detection failed during reconciliation.
	DriftDetectionFailedReason = "DriftDetectionFailed"

	// HealthChecksPassedReason represents the fact that one or more
	// health checks failed during reconciliation.
	HealthChecksFailedReason = "HealthChecksFailed"

	// NoDriftReason represents the fact that during reconcilliation
	// no drift was detected.
	NoDriftReason = "NoDrift"

	// OutputsWritingFailedReason represents the fact that writing
	// outputs for the Terraform resource status failed.
	OutputsWritingFailedReason = "OutputsWritingFailed"

	// PlannedNoChangesReason represents the fact that Terraform
	// planned no changes during reconciliation.
	PlannedNoChangesReason = "TerraformPlannedNoChanges"

	// PlannedWithChangesReason represents the fact that Terraform
	// planned changes during reconciliation.
	PlannedWithChangesReason = "TerraformPlannedWithChanges"

	// PostPlanningWebhookFailedReason represents the fact that
	// the post-planning webhook failed during reconciliation.
	PostPlanningWebhookFailedReason = "PostPlanningWebhookFailed"

	// TFExecApplyFailedReason represents the fact that the execution
	// of 'terraform apply' failed.
	TFExecApplyFailedReason = "TFExecApplyFailed"

	// TFExecApplySucceedReason represents the fact that the execution
	// of 'terraform apply' succeeded.
	TFExecApplySucceedReason = "TerraformAppliedSucceed"

	// TFExecForceUnlockReason represents the fact that the controller
	// is attempting to force unlock the Terraform state.
	TFExecForceUnlockReason = "ForceUnlock"

	// TFExecInitFailedReason represents the fact that the an error
	// occured while initializing Terraform.
	TFExecInitFailedReason = "TFExecInitFailed"

	// TFExecLockHeldReason represents the fact that the Terraform
	// state lock is held by another process.
	TFExecLockHeldReason = "LockHeld"

	// TFExecNewFailedReason represents the fact that the creation
	// of the Terraform process failed.
	TFExecNewFailedReason = "TFExecNewFailed"

	// TFExecOutputFailedReason represents the fact that the execution
	// of 'terraform output' failed.
	TFExecOutputFailedReason = "TFExecOutputFailed"

	// TFExecPlanFailedReason represents the fact that the execution
	// of 'terraform plan' failed.
	TFExecPlanFailedReason = "TFExecPlanFailed"

	// TemplateGenerationFailedReason represents the fact that
	// the generation of the Terraform .tf template failed.
	TemplateGenerationFailedReason = "TemplateGenerationFailed"

	// VarsGenerationFailedReason represents the fact that
	// the generation of the Terraform variables failed.
	VarsGenerationFailedReason = "VarsGenerationFailed"

	// WorkspaceSelectFailedReason represents the fact that selecting
	// a Terraform workspace failed.
	WorkspaceSelectFailedReason = "SelectWorkspaceFailed"
)
