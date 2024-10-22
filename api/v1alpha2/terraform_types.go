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

import (
	"bytes"
	"fmt"
	"math"
	"net"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	"github.com/flux-iac/tofu-controller/api/planid"
)

const (
	CACertSecretName = "tf-controller.tls"
	// RunnerTLSSecretName is the name of the secret containing a TLS cert that will be written to
	// the namespace in which a terraform runner is created
	RunnerTLSSecretName     = "terraform-runner.tls"
	RunnerLabel             = "infra.contrib.fluxcd.io/terraform"
	GitRepositoryIndexKey   = ".metadata.gitRepository"
	BucketIndexKey          = ".metadata.bucket"
	OCIRepositoryIndexKey   = ".metadata.ociRepository"
	BreakTheGlassAnnotation = "break-the-glass.tf-controller/requestedAt"
)

type ReadInputsFromSecretSpec struct {
	// +required
	Name string `json:"name"`

	// +required
	As string `json:"as"`
}

// WriteOutputsToSecretSpec defines where to store outputs, and which outputs to be stored.
type WriteOutputsToSecretSpec struct {
	// Name is the name of the Secret to be written
	// +required
	Name string `json:"name"`

	// Labels to add to the outputted secret
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations to add to the outputted secret
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Outputs contain the selected names of outputs to be written
	// to the secret. Empty array means writing all outputs, which is default.
	// +optional
	Outputs []string `json:"outputs,omitempty"`
}

type Variable struct {
	// Name is the name of the variable
	// +required
	Name string `json:"name"`

	// +optional
	Value *apiextensionsv1.JSON `json:"value,omitempty"`

	// +optional
	ValueFrom *corev1.EnvVarSource `json:"valueFrom,omitempty"`
}

// TerraformSpec defines the desired state of Terraform
type TerraformSpec struct {

	// ApprovePlan specifies name of a plan wanted to approve.
	// If its value is "auto", the controller will automatically approve every plan.
	// +optional
	ApprovePlan string `json:"approvePlan,omitempty"`

	// Destroy produces a destroy plan. Applying the plan will destroy all resources.
	// +optional
	Destroy bool `json:"destroy,omitempty"`

	// +optional
	BackendConfig *BackendConfigSpec `json:"backendConfig,omitempty"`

	// +optional
	BackendConfigsFrom []BackendConfigsReference `json:"backendConfigsFrom,omitempty"`

	// +optional
	Cloud *CloudSpec `json:"cloud,omitempty"`

	// +optional
	// +kubebuilder:default:=default
	Workspace string `json:"workspace,omitempty"`

	// List of input variables to set for the Terraform program.
	// +optional
	Vars []Variable `json:"vars,omitempty"`

	// List of references to a Secret or a ConfigMap to generate variables for
	// Terraform resources based on its data, selectively by varsKey. Values of the later
	// Secret / ConfigMap with the same keys will override those of the former.
	// +optional
	VarsFrom []VarsReference `json:"varsFrom,omitempty"`

	// Values map to the Terraform variable "values", which is an object of arbitrary values.
	// It is a convenient way to pass values to Terraform resources without having to define
	// a variable for each value. To use this feature, your Terraform file must define the variable "values".
	// +optional
	Values *apiextensionsv1.JSON `json:"values,omitempty"`

	// TfVarsFiles loads all given .tfvars files. It copycats the -var-file functionality.
	// +optional
	TfVarsFiles []string `json:"tfVarsFiles,omitempty"`

	// List of all configuration files to be created in initialization.
	// +optional
	FileMappings []FileMapping `json:"fileMappings,omitempty"`

	// The interval at which to reconcile the Terraform.
	// +required
	Interval metav1.Duration `json:"interval"`

	// The interval at which to retry a previously failed reconciliation.
	// The default value is 15 when not specified.
	// +optional
	RetryInterval *metav1.Duration `json:"retryInterval,omitempty"`

	// The strategy to use when retrying a previously failed reconciliation.
	// The default strategy is StaticInterval and the retry interval is based on the RetryInterval value.
	// The ExponentialBackoff strategy uses the formula: 2^reconciliationFailures * RetryInterval with a
	// maximum requeue duration of MaxRetryInterval.
	// +kubebuilder:validation:Enum=StaticInterval;ExponentialBackoff
	// +kubebuilder:default:string=StaticInterval
	// +optional
	RetryStrategy RetryStrategyEnum `json:"retryStrategy,omitempty"`

	// The maximum requeue duration after  a previously failed reconciliation.
	// Only applicable when RetryStrategy is set to ExponentialBackoff.
	// The default value is 24 hours when not specified.
	// +optional
	MaxRetryInterval *metav1.Duration `json:"maxRetryInterval,omitempty"`

	// Path to the directory containing Terraform (.tf) files.
	// Defaults to 'None', which translates to the root path of the SourceRef.
	// +optional
	Path string `json:"path,omitempty"`

	// SourceRef is the reference of the source where the Terraform files are stored.
	// +required
	SourceRef CrossNamespaceSourceReference `json:"sourceRef"`

	// Suspend is to tell the controller to suspend subsequent TF executions,
	// it does not apply to already started executions. Defaults to false.
	// +optional
	Suspend bool `json:"suspend,omitempty"`

	// Force instructs the controller to unconditionally
	// re-plan and re-apply TF resources. Defaults to false.
	// +kubebuilder:default:=false
	// +optional
	Force bool `json:"force,omitempty"`

	// +optional
	ReadInputsFromSecrets []ReadInputsFromSecretSpec `json:"readInputsFromSecrets,omitempty"`

	// A list of target secrets for the outputs to be written as.
	// +optional
	WriteOutputsToSecret *WriteOutputsToSecretSpec `json:"writeOutputsToSecret,omitempty"`

	// Disable automatic drift detection. Drift detection may be resource intensive in
	// the context of a large cluster or complex Terraform statefile. Defaults to false.
	// +kubebuilder:default:=false
	// +optional
	DisableDriftDetection bool `json:"disableDriftDetection,omitempty"`

	// +optional
	// PushSpec *PushSpec `json:"pushSpec,omitempty"`

	// +optional
	CliConfigSecretRef *corev1.SecretReference `json:"cliConfigSecretRef,omitempty"`

	// List of health checks to be performed.
	// +optional
	HealthChecks []HealthCheck `json:"healthChecks,omitempty"`

	// Create destroy plan and apply it to destroy terraform resources
	// upon deletion of this object. Defaults to false.
	// +kubebuilder:default:=false
	// +optional
	DestroyResourcesOnDeletion bool `json:"destroyResourcesOnDeletion,omitempty"`

	// Name of a ServiceAccount for the runner Pod to provision Terraform resources.
	// Default to tf-runner.
	// +kubebuilder:default:=tf-runner
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// Clean the runner pod up after each reconciliation cycle
	// +kubebuilder:default:=true
	// +optional
	AlwaysCleanupRunnerPod *bool `json:"alwaysCleanupRunnerPod,omitempty"`

	// Configure the termination grace period for the runner pod. Use this parameter
	// to allow the Terraform process to gracefully shutdown. Consider increasing for
	// large, complex or slow-moving Terraform managed resources.
	// +kubebuilder:default:=30
	// +optional
	RunnerTerminationGracePeriodSeconds *int64 `json:"runnerTerminationGracePeriodSeconds,omitempty"`

	// UpgradeOnInit configures to upgrade modules and providers on initialization of a stack
	// +kubebuilder:default:=true
	// +optional
	UpgradeOnInit bool `json:"upgradeOnInit,omitempty"`

	// RefreshBeforeApply forces refreshing of the state before the apply step.
	// +kubebuilder:default:=false
	// +optional
	RefreshBeforeApply bool `json:"refreshBeforeApply,omitempty"`

	// +optional
	RunnerPodTemplate RunnerPodTemplate `json:"runnerPodTemplate,omitempty"`

	// EnableInventory enables the object to store resource entries as the inventory for external use.
	// +optional
	EnableInventory bool `json:"enableInventory,omitempty"`

	// +optional
	TFState *TFStateSpec `json:"tfstate,omitempty"`

	// Targets specify the resource, module or collection of resources to target.
	// +optional
	Targets []string `json:"targets,omitempty"`

	// Parallelism limits the number of concurrent operations of Terraform apply step. Zero (0) means using the default value.
	// +kubebuilder:default:=0
	// +optional
	Parallelism int32 `json:"parallelism,omitempty"`

	// StoreReadablePlan enables storing the plan in a readable format.
	// +kubebuilder:validation:Enum=none;json;human
	// +kubebuilder:default:=none
	// +optional
	StoreReadablePlan string `json:"storeReadablePlan,omitempty"`

	// +optional
	Webhooks []Webhook `json:"webhooks,omitempty"`

	// +optional
	DependsOn []meta.NamespacedObjectReference `json:"dependsOn,omitempty"`

	// Enterprise is the enterprise configuration placeholder.
	// +optional
	Enterprise *apiextensionsv1.JSON `json:"enterprise,omitempty"`

	// PlanOnly specifies if the reconciliation should or should not stop at plan
	// phase.
	// +optional
	PlanOnly bool `json:"planOnly,omitempty"`

	// BreakTheGlass specifies if the reconciliation should stop
	// and allow interactive shell in case of emergency.
	// +optional
	BreakTheGlass bool `json:"breakTheGlass,omitempty"`

	// BranchPlanner configuration.
	// +optional
	BranchPlanner *BranchPlanner `json:"branchPlanner,omitempty"`

	// Remediation specifies what the controller should do when reconciliation
	// fails. The default is to not perform any action.
	// +optional
	Remediation *Remediation `json:"remediation,omitempty"`
}

type BranchPlanner struct {
	// EnablePathScope specifies if the Branch Planner should or shouldn't check
	// if a Pull Request has changes under `.spec.path`. If enabled extra
	// resources will be created only if there are any changes in terraform files.
	// +optional
	EnablePathScope bool `json:"enablePathScope"`
}

type Remediation struct {
	// Retries is the number of retries that should be attempted on failures
	// before bailing. Defaults to '0', a negative integer denotes unlimited
	// retries.
	// +optional
	Retries int64 `json:"retries,omitempty"`
}

type CloudSpec struct {
	// +required
	Organization string `json:"organization"`

	// +required
	Workspaces *CloudWorkspacesSpec `json:"workspaces"`

	// +optional
	Hostname string `json:"hostname,omitempty"`

	// +optional
	Token string `json:"token,omitempty"`
}

type CloudWorkspacesSpec struct {
	// +optional
	Name string `json:"name"`

	// +optional
	Tags []string `json:"tags,omitempty"`
}

type Webhook struct {
	// +kubebuilder:validation:Enum=post-planning
	// +kubebuilder:default:=post-planning
	// +required
	Stage string `json:"stage"`

	// +kubebuilder:default:=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// +required
	URL string `json:"url"`

	// +kubebuilder:value:Enum=SpecAndPlan,SpecOnly,PlanOnly
	// +kubebuilder:default:=SpecAndPlan
	// +optional
	PayloadType string `json:"payloadType,omitempty"`

	// +optional
	ErrorMessageTemplate string `json:"errorMessageTemplate,omitempty"`

	// +required
	TestExpression string `json:"testExpression,omitempty"`
}

func (w Webhook) IsEnabled() bool {
	return w.Enabled == nil || *w.Enabled
}

type PlanStatus struct {
	// +optional
	LastApplied string `json:"lastApplied,omitempty"`

	// +optional
	Pending string `json:"pending,omitempty"`

	// +optional
	IsDestroyPlan bool `json:"isDestroyPlan,omitempty"`

	// +optional
	IsDriftDetectionPlan bool `json:"isDriftDetectionPlan,omitempty"`
}

// TerraformStatus defines the observed state of Terraform
type TerraformStatus struct {
	meta.ReconcileRequestStatus `json:",inline"`

	// ObservedGeneration is the last reconciled generation.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// The last successfully applied revision.
	// The revision format for Git sources is <branch|tag>/<commit-sha>.
	// +optional
	LastAppliedRevision string `json:"lastAppliedRevision,omitempty"`

	// LastAttemptedRevision is the revision of the last reconciliation attempt.
	// +optional
	LastAttemptedRevision string `json:"lastAttemptedRevision,omitempty"`

	// LastPlannedRevision is the revision used by the last planning process.
	// The result could be either no plan change or a new plan generated.
	// +optional
	LastPlannedRevision string `json:"lastPlannedRevision,omitempty"`

	// LastPlanAt is the time when the last terraform plan was performed
	// +optional
	LastPlanAt *metav1.Time `json:"lastPlanAt,omitempty"`

	// LastDriftDetectedAt is the time when the last drift was detected
	// +optional
	LastDriftDetectedAt *metav1.Time `json:"lastDriftDetectedAt,omitempty"`

	// LastAppliedByDriftDetectionAt is the time when the last drift was detected and
	// terraform apply was performed as a result
	// +optional
	LastAppliedByDriftDetectionAt *metav1.Time `json:"lastAppliedByDriftDetectionAt,omitempty"`

	// +optional
	AvailableOutputs []string `json:"availableOutputs,omitempty"`

	// +optional
	Plan PlanStatus `json:"plan,omitempty"`

	// Inventory contains the list of Terraform resource object references that have been successfully applied.
	// +optional
	Inventory *ResourceInventory `json:"inventory,omitempty"`

	// +optional
	Lock LockStatus `json:"lock,omitempty"`

	// ReconciliationFailures is the number of reconciliation
	// failures since the last success or update.
	// +optional
	ReconciliationFailures int64 `json:"reconciliationFailures,omitempty"`
}

// LockStatus defines the observed state of a Terraform State Lock
type LockStatus struct {
	// +optional
	LastApplied string `json:"lastApplied,omitempty"`

	// Pending holds the identifier of the Lock Holder to be used with Force Unlock
	// +optional
	Pending string `json:"pending,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=tf
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status",description=""
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].message",description=""
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description=""

// Terraform is the Schema for the terraforms API
type Terraform struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec TerraformSpec `json:"spec,omitempty"`
	// +kubebuilder:default={"observedGeneration":-1}
	Status TerraformStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TerraformList contains a list of Terraform
type TerraformList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Terraform `json:"items"`
}

// BackendConfigSpec is for specifying configuration for Terraform's Kubernetes backend
type BackendConfigSpec struct {

	// Disable is to completely disable the backend configuration.
	// +optional
	Disable bool `json:"disable"`

	// +optional
	SecretSuffix string `json:"secretSuffix,omitempty"`

	// +optional
	InClusterConfig bool `json:"inClusterConfig,omitempty"`

	// +optional
	CustomConfiguration string `json:"customConfiguration,omitempty"`

	// +optional
	ConfigPath string `json:"configPath,omitempty"`

	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// TFStateSpec allows the user to set ForceUnlock
type TFStateSpec struct {
	// ForceUnlock a Terraform state if it has become locked for any reason. Defaults to `no`.
	//
	// This is an Enum and has the expected values of:
	//
	// - auto
	// - yes
	// - no
	//
	// WARNING: Only use `auto` in the cases where you are absolutely certain that
	// no other system is using this state, you could otherwise end up in a bad place
	// See https://www.terraform.io/language/state/locking#force-unlock for more
	// information on the terraform state lock and force unlock.
	//
	// +optional
	// +kubebuilder:validation:Enum:=yes;no;auto
	// +kubebuilder:default:string=no
	ForceUnlock ForceUnlockEnum `json:"forceUnlock,omitempty"`

	// LockIdentifier holds the Identifier required by Terraform to unlock the state
	// if it ever gets into a locked state.
	//
	// You'll need to put the Lock Identifier in here while setting ForceUnlock to
	// either `yes` or `auto`.
	//
	// Leave this empty to do nothing, set this to the value of the `Lock Info: ID: [value]`,
	// e.g. `f2ab685b-f84d-ac0b-a125-378a22877e8d`, to force unlock the state.
	//
	// +optional
	LockIdentifier string `json:"lockIdentifier,omitempty"`

	// LockTimeout is a Duration string that instructs Terraform to retry acquiring a lock for the specified period of
	// time before returning an error. The duration syntax is a number followed by a time unit letter, such as `3s` for
	// three seconds.
	//
	// Defaults to `0s` which will behave as though `LockTimeout` was not set
	//
	// +optional
	// +kubebuilder:default:string="0s"
	LockTimeout metav1.Duration `json:"lockTimeout,omitempty"`
}

type ForceUnlockEnum string

const (
	ForceUnlockEnumAuto ForceUnlockEnum = "auto"
	ForceUnlockEnumYes  ForceUnlockEnum = "yes"
	ForceUnlockEnumNo   ForceUnlockEnum = "no"
)

type RetryStrategyEnum string

const (
	StaticInterval     RetryStrategyEnum = "StaticInterval"
	ExponentialBackoff RetryStrategyEnum = "ExponentialBackoff"
)

const (
	TerraformKind             = "Terraform"
	TerraformFinalizer        = "finalizers.tf.contrib.fluxcd.io"
	MaxConditionMessageLength = 20000
	DisabledValue             = "disabled"
	ApprovePlanAutoValue      = "auto"
	ApprovePlanDisableValue   = "disable"
	DefaultWorkspaceName      = "default"
)

// The potential reasons that are associated with condition types
const (
	AccessDeniedReason              = "AccessDenied"
	ArtifactFailedReason            = "ArtifactFailed"
	RetryLimitReachedReason         = "RetryLimitReached"
	DeletionBlockedByDependants     = "DeletionBlockedByDependantsReason"
	DependencyNotReadyReason        = "DependencyNotReady"
	DriftDetectedReason             = "DriftDetected"
	DriftDetectionFailedReason      = "DriftDetectionFailed"
	HealthChecksFailedReason        = "HealthChecksFailed"
	NoDriftReason                   = "NoDrift"
	OutputsWritingFailedReason      = "OutputsWritingFailed"
	PlannedNoChangesReason          = "TerraformPlannedNoChanges"
	PlannedWithChangesReason        = "TerraformPlannedWithChanges"
	PostPlanningWebhookFailedReason = "PostPlanningWebhookFailed"
	TFExecApplyFailedReason         = "TFExecApplyFailed"
	TFExecApplySucceedReason        = "TerraformAppliedSucceed"
	TFExecForceUnlockReason         = "ForceUnlock"
	TFExecInitFailedReason          = "TFExecInitFailed"
	TFExecLockHeldReason            = "LockHeld"
	TFExecNewFailedReason           = "TFExecNewFailed"
	TFExecOutputFailedReason        = "TFExecOutputFailed"
	TFExecPlanFailedReason          = "TFExecPlanFailed"
	TemplateGenerationFailedReason  = "TemplateGenerationFailed"
	VarsGenerationFailedReason      = "VarsGenerationFailed"
	WorkspaceSelectFailedReason     = "SelectWorkspaceFailed"
)

// These constants are the Condition Types that the Terraform Resource works with
const (
	ConditionTypeApply       = "Apply"
	ConditionTypeHealthCheck = "HealthCheck"
	ConditionTypeOutput      = "Output"
	ConditionTypePlan        = "Plan"
	ConditionTypeStateLocked = "StateLocked"
)

// Webhook stages
const (
	PostPlanningWebhook = "post-planning"
)

const (
	TFDependencyOfPrefix = "tf.dependency.of."
)

// SetTerraformReadiness sets the ReadyCondition, ObservedGeneration, and LastAttemptedRevision, on the Terraform.
func SetTerraformReadiness(terraform *Terraform, status metav1.ConditionStatus, reason, message string, revision string) {
	newCondition := metav1.Condition{
		Type:    meta.ReadyCondition,
		Status:  status,
		Reason:  reason,
		Message: trimString(message, MaxConditionMessageLength),
	}

	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)
	terraform.Status.ObservedGeneration = terraform.Generation
	terraform.Status.LastAttemptedRevision = revision
}

func TerraformApplying(terraform Terraform, revision string, message string) Terraform {
	newCondition := metav1.Condition{
		Type:    ConditionTypeApply,
		Status:  metav1.ConditionUnknown,
		Reason:  meta.ProgressingReason,
		Message: trimString(message, MaxConditionMessageLength),
	}
	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)
	if revision != "" {
		terraform.Status.LastAttemptedRevision = revision
	}
	return terraform
}

func TerraformOutputsAvailable(terraform Terraform, availableOutputs []string, message string) Terraform {
	newCondition := metav1.Condition{
		Type:    ConditionTypeOutput,
		Status:  metav1.ConditionTrue,
		Reason:  "TerraformOutputsAvailable",
		Message: trimString(message, MaxConditionMessageLength),
	}
	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)
	terraform.Status.AvailableOutputs = availableOutputs
	return terraform
}

func TerraformOutputsWritten(terraform Terraform, revision string, message string) Terraform {
	newCondition := metav1.Condition{
		Type:    ConditionTypeOutput,
		Status:  metav1.ConditionTrue,
		Reason:  "TerraformOutputsWritten",
		Message: trimString(message, MaxConditionMessageLength),
	}
	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)

	SetTerraformReadiness(&terraform, metav1.ConditionTrue, "TerraformOutputsWritten", message+": "+revision, revision)
	return terraform
}

func TerraformApplied(terraform Terraform, revision string, message string, isDestroyApply bool, entries []ResourceRef) Terraform {
	newCondition := metav1.Condition{
		Type:    ConditionTypeApply,
		Status:  metav1.ConditionTrue,
		Reason:  TFExecApplySucceedReason,
		Message: trimString(message, MaxConditionMessageLength),
	}
	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)

	if terraform.Status.Plan.IsDriftDetectionPlan {
		terraform.Status.LastAppliedByDriftDetectionAt = &metav1.Time{Time: time.Now()}
	}

	terraform.Status.Plan = PlanStatus{
		LastApplied:   terraform.Status.Plan.Pending,
		Pending:       "",
		IsDestroyPlan: isDestroyApply,
	}
	if revision != "" {
		terraform.Status.LastAppliedRevision = revision
	}

	if len(entries) > 0 {
		terraform.Status.Inventory = &ResourceInventory{Entries: entries}
	}

	SetTerraformReadiness(&terraform, metav1.ConditionUnknown, TFExecApplySucceedReason, message+": "+revision, revision)
	return terraform
}

func TerraformPostPlanningWebhookFailed(terraform Terraform, revision string, message string) Terraform {
	newCondition := metav1.Condition{
		Type:    ConditionTypePlan,
		Status:  metav1.ConditionFalse,
		Reason:  PostPlanningWebhookFailedReason,
		Message: trimString(message, MaxConditionMessageLength),
	}
	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)
	terraform.Status.Plan = PlanStatus{
		LastApplied:   terraform.Status.Plan.LastApplied,
		Pending:       "",
		IsDestroyPlan: terraform.Spec.Destroy,
	}
	if revision != "" {
		terraform.Status.LastAttemptedRevision = revision
		terraform.Status.LastPlannedRevision = revision
	}

	return terraform
}

func TerraformPlannedWithChanges(terraform Terraform, revision string, forceOrAutoApply bool, message string) Terraform {
	planId := planid.GetPlanID(revision)
	approveMessage := planid.GetApproveMessage(planId, message)

	newCondition := metav1.Condition{
		Type:    ConditionTypePlan,
		Status:  metav1.ConditionTrue,
		Reason:  PlannedWithChangesReason,
		Message: trimString(message, MaxConditionMessageLength),
	}
	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)
	terraform.Status.Plan = PlanStatus{
		LastApplied:          terraform.Status.Plan.LastApplied,
		Pending:              planId, // pending plan id is always the short plan format.
		IsDestroyPlan:        terraform.Spec.Destroy,
		IsDriftDetectionPlan: terraform.HasDrift(),
	}
	if revision != "" {
		terraform.Status.LastAttemptedRevision = revision
		terraform.Status.LastPlannedRevision = revision
	}

	terraform.Status.LastPlanAt = &metav1.Time{Time: time.Now()}

	// planOnly takes the highest precedence
	if terraform.Spec.PlanOnly {
		SetTerraformReadiness(&terraform, metav1.ConditionUnknown, PlannedWithChangesReason, message+": This object is in the plan only mode.", revision)
	} else if forceOrAutoApply {
		SetTerraformReadiness(&terraform, metav1.ConditionUnknown, PlannedWithChangesReason, message, revision)
	} else {
		// this is the manual mode, where we don't want to apply the plan
		SetTerraformReadiness(&terraform, metav1.ConditionUnknown, PlannedWithChangesReason, approveMessage, revision)
	}
	return terraform
}

func TerraformPlannedNoChanges(terraform Terraform, revision string, message string) Terraform {
	newCondition := metav1.Condition{
		Type:    ConditionTypePlan,
		Status:  metav1.ConditionFalse,
		Reason:  PlannedNoChangesReason,
		Message: trimString(message, MaxConditionMessageLength),
	}
	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)
	terraform.Status.Plan = PlanStatus{
		LastApplied:   terraform.Status.Plan.LastApplied,
		Pending:       "",
		IsDestroyPlan: terraform.Spec.Destroy,
	}
	if revision != "" {
		terraform.Status.LastAttemptedRevision = revision
		terraform.Status.LastPlannedRevision = revision
	}

	terraform.Status.LastPlanAt = &metav1.Time{Time: time.Now()}

	SetTerraformReadiness(&terraform, metav1.ConditionTrue, PlannedNoChangesReason, message+": "+revision, revision)
	return terraform
}

// TerraformProgressing resets the conditions of the given Terraform to a single
// ReadyCondition with status ConditionUnknown.
func TerraformProgressing(terraform Terraform, message string) Terraform {
	newCondition := metav1.Condition{
		Type:    meta.ReadyCondition,
		Status:  metav1.ConditionUnknown,
		Reason:  meta.ProgressingReason,
		Message: trimString(message, MaxConditionMessageLength),
	}
	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)
	return terraform
}

// TerraformNotReady registers a failed apply attempt of the given Terraform.
func TerraformNotReady(terraform Terraform, revision, reason, message string) Terraform {
	SetTerraformReadiness(&terraform, metav1.ConditionFalse, reason, trimString(message, MaxConditionMessageLength), revision)
	if revision != "" {
		terraform.Status.LastAttemptedRevision = revision
	}
	return terraform
}

func TerraformAppliedFailResetPlanAndNotReady(terraform Terraform, revision, reason, message string) Terraform {
	newCondition := metav1.Condition{
		Type:    ConditionTypeApply,
		Status:  metav1.ConditionFalse,
		Reason:  "TerraformAppliedFail",
		Message: trimString(message, MaxConditionMessageLength),
	}
	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)
	terraform = TerraformNotReady(terraform, revision, reason, message)
	terraform.Status.Plan.Pending = ""
	return terraform
}

func TerraformDriftDetected(terraform Terraform, revision, reason, message string) Terraform {
	terraform.Status.LastDriftDetectedAt = &metav1.Time{Time: time.Now()}

	SetTerraformReadiness(&terraform, metav1.ConditionFalse, reason, trimString(message, MaxConditionMessageLength), revision)
	return terraform
}

func TerraformNoDrift(terraform Terraform, revision, reason, message string) Terraform {
	SetTerraformReadiness(&terraform, metav1.ConditionTrue, reason, message+": "+revision, revision)
	return terraform
}

func TerraformHealthCheckFailed(terraform Terraform, message string) Terraform {
	newCondition := metav1.Condition{
		Type:    ConditionTypeHealthCheck,
		Status:  metav1.ConditionFalse,
		Reason:  HealthChecksFailedReason,
		Message: trimString(message, MaxConditionMessageLength),
	}
	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)
	return terraform
}

func TerraformHealthCheckSucceeded(terraform Terraform, message string) Terraform {
	newCondition := metav1.Condition{
		Type:    ConditionTypeHealthCheck,
		Status:  metav1.ConditionTrue,
		Reason:  "HealthChecksSucceed",
		Message: trimString(message, MaxConditionMessageLength),
	}
	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)
	return terraform
}

// TerraformForceUnlock will set a new condition on the Terraform resource indicating
// that we are attempting to force unlock it.
func TerraformForceUnlock(terraform Terraform, message string) Terraform {
	newCondition := metav1.Condition{
		Type:    ConditionTypeStateLocked,
		Status:  metav1.ConditionFalse,
		Reason:  TFExecForceUnlockReason,
		Message: trimString(message, MaxConditionMessageLength),
	}
	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)

	if terraform.Status.Lock.Pending != "" && terraform.Status.Lock.LastApplied != terraform.Status.Lock.Pending {
		terraform.Status.Lock.LastApplied = terraform.Status.Lock.Pending
	}

	terraform.Status.Lock.Pending = ""
	return terraform
}

// TerraformStateLocked will set a new condition on the Terraform resource indicating
// that the resource has been locked.
func TerraformStateLocked(terraform Terraform, lockID, message string) Terraform {
	newCondition := metav1.Condition{
		Type:    ConditionTypeStateLocked,
		Status:  metav1.ConditionTrue,
		Reason:  TFExecLockHeldReason,
		Message: trimString(message, MaxConditionMessageLength),
	}
	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)
	SetTerraformReadiness(&terraform, metav1.ConditionFalse, newCondition.Reason, newCondition.Message, "")

	if terraform.Status.Lock.Pending != "" && terraform.Status.Lock.LastApplied != terraform.Status.Lock.Pending {
		terraform.Status.Lock.LastApplied = terraform.Status.Lock.Pending
	}

	terraform.Status.Lock.Pending = lockID
	return terraform
}

// TerraformReachedLimit will set a new condition on the Terraform resource
// indicating that the resource has reached its retry limit.
func TerraformReachedLimit(terraform Terraform) Terraform {
	newCondition := metav1.Condition{
		Type:    meta.StalledCondition,
		Status:  metav1.ConditionTrue,
		Reason:  RetryLimitReachedReason,
		Message: "Resource reached maximum number of retries.",
	}
	apimeta.SetStatusCondition(terraform.GetStatusConditions(), newCondition)

	return terraform
}

// TerraformResetRetry will set a new condition on the Terraform resource
// indicating that the resource retry count has been reset.
func TerraformResetRetry(terraform Terraform) Terraform {
	apimeta.RemoveStatusCondition(terraform.GetStatusConditions(), meta.StalledCondition)
	terraform.resetReconciliationFailures()

	return terraform
}

// HasDrift returns true if drift has been detected since the last successful apply
func (in Terraform) HasDrift() bool {
	for _, condition := range in.Status.Conditions {
		if condition.Type == ConditionTypeApply &&
			condition.Status == metav1.ConditionTrue &&
			in.Status.LastDriftDetectedAt != nil &&
			(*in.Status.LastDriftDetectedAt).After(condition.LastTransitionTime.Time) {
			return true
		}
	}
	return false
}

// GetDependsOn returns the list of dependencies, namespace scoped.
func (in Terraform) GetDependsOn() []meta.NamespacedObjectReference {
	return in.Spec.DependsOn
}

// GetRetryInterval returns the retry interval
func (in Terraform) GetRetryInterval() time.Duration {
	retryInterval := 15 * time.Second
	if in.Spec.RetryInterval != nil {
		retryInterval = in.Spec.RetryInterval.Duration
	}

	if in.Spec.RetryStrategy == ExponentialBackoff {
		retryInterval *= time.Duration(math.Pow(2, float64(in.Status.ReconciliationFailures)))
		maxRetryInterval := 24 * time.Hour
		if in.Spec.MaxRetryInterval != nil {
			maxRetryInterval = in.Spec.MaxRetryInterval.Duration
		}

		if retryInterval > maxRetryInterval {
			return maxRetryInterval
		}
	}

	return retryInterval
}

// GetStatusConditions returns a pointer to the Status.Conditions slice.
func (in *Terraform) GetStatusConditions() *[]metav1.Condition {
	return &in.Status.Conditions
}

// GetConditions returns a pointer to the Status.Conditions slice.
// pretty much the same as GetStatusConditions but to comply with flux conditions.Getter interface
// it needs to return a copy of the conditions slice
func (in Terraform) GetConditions() []metav1.Condition {
	return in.Status.Conditions
}

func (in *Terraform) WorkspaceName() string {
	if in.Spec.Workspace != "" {
		return in.Spec.Workspace
	}
	return DefaultWorkspaceName
}

func (in Terraform) ToBytes(scheme *runtime.Scheme) ([]byte, error) {
	return runtime.Encode(
		serializer.NewCodecFactory(scheme).LegacyCodec(
			corev1.SchemeGroupVersion,
			GroupVersion,
			sourcev1.GroupVersion,
		), &in)
}

func (in *Terraform) FromBytes(b []byte, scheme *runtime.Scheme) error {
	return runtime.DecodeInto(
		serializer.NewCodecFactory(scheme).LegacyCodec(
			corev1.SchemeGroupVersion,
			GroupVersion,
			sourcev1.GroupVersion,
		), b, in)
}

func (in *Terraform) GetRunnerHostname(target string, clusterDomain string) string {
	if net.ParseIP(target) != nil {
		prefix := strings.ReplaceAll(target, ".", "-")
		return fmt.Sprintf("%s.%s.pod.%s", prefix, in.Namespace, clusterDomain)
	} else {
		return fmt.Sprintf("%s.tf-runner.%s.svc.%s", target, in.Namespace, clusterDomain)
	}
}

func (in *Terraform) GetRetries() int64 {
	if in.Spec.Remediation == nil {
		return 0
	}

	return in.Spec.Remediation.Retries
}

func (in *Terraform) GetReconciliationFailures() int64 {
	return in.Status.ReconciliationFailures
}

func (in *Terraform) IncrementReconciliationFailures() {
	in.Status.ReconciliationFailures++
}

func (in *Terraform) resetReconciliationFailures() {
	in.Status.ReconciliationFailures = 0
}

func (in *Terraform) ShouldRetry() bool {
	if in.Spec.Remediation == nil || in.Spec.Remediation.Retries < 0 {
		return true
	}

	return in.GetReconciliationFailures() < in.Spec.Remediation.Retries
}

func (in *TerraformSpec) GetAlwaysCleanupRunnerPod() bool {
	if in.AlwaysCleanupRunnerPod == nil {
		return true
	}

	return *in.AlwaysCleanupRunnerPod
}

func (c *CloudSpec) IsValid() bool {
	if c.Organization == "" {
		return false
	}

	if c.Workspaces == nil {
		return false
	}

	if c.Workspaces.Name == "" && c.Workspaces.Tags == nil {
		return false
	}

	return true
}

func (c *CloudSpec) ToHCL() string {
	var buf bytes.Buffer
	buf.WriteString("terraform {\n")
	buf.WriteString("  cloud {\n")
	buf.WriteString(fmt.Sprintf("    organization = %q\n", c.Organization))
	buf.WriteString(fmt.Sprintf("    workspaces {\n"))
	if c.Workspaces.Name != "" {
		buf.WriteString(fmt.Sprintf("      name = %q\n", c.Workspaces.Name))
	}
	if len(c.Workspaces.Tags) > 0 {
		tags := "[\"" + strings.Join(c.Workspaces.Tags, "\", \"") + "\"]"
		buf.WriteString(fmt.Sprintf("      tags = %s\n", tags))
	}
	buf.WriteString(fmt.Sprintf("    }\n"))
	buf.WriteString(fmt.Sprintf("    hostname = %q\n", c.Hostname))
	buf.WriteString(fmt.Sprintf("    token = %q\n", c.Token))
	buf.WriteString(fmt.Sprintf("  }\n"))
	buf.WriteString(fmt.Sprintf("}\n"))

	return buf.String()
}

// trimString takes in a string and an integer limit, and returns a new string with a maximum length of limit characters.
// If the length of the input string is greater than limit, the returned string will be truncated to limit characters
// and "..." will be appended to the end.
// If limit is less than 3, it will be set to 3 before continuing.
// It correctly handles unicode characters by using utf8.RuneCountInString to get the number of runes in the string.
func trimString(str string, limit int) string {
	if limit < 3 {
		limit = 3
	}
	if utf8.RuneCountInString(str) <= limit {
		return str
	}

	runes := []rune(str)
	return string(runes[:limit]) + "..."
}

func init() {
	SchemeBuilder.Register(&Terraform{}, &TerraformList{})
}
