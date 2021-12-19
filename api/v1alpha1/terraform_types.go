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

package v1alpha1

import (
	"fmt"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

/*
  writeOutputsToSecret:
    name: helloworld-tf-outputs
    outputs:
    - hello_world
*/
// WriteOutputsToSecretSpec defines where to store outputs, and which outputs to be stored.
type WriteOutputsToSecretSpec struct {
	// Secret name
	Name string `json:"name"`

	// Empty list means all
	Outputs []string `json:"outputs"`
}

type Variable struct {
	Name string `json:"name"`

	// +optional
	Value string `json:"value,omitempty"`

	// +optional
	ValueFrom *v1.EnvVarSource `json:"valueFrom,omitempty"`
}

// TerraformSpec defines the desired state of Terraform
type TerraformSpec struct {

	// ApprovePlan specifies name of a plan wanted to approve.
	// If its value is "auto", the controller will automatically approve every plan.
	// +optional
	ApprovePlan string `json:"approvePlan,omitempty"`

	// +optional
	BackendConfig *BackendConfigSpec `json:"backendConfig,omitempty"`

	// List of input variables to set for the Terraform program.
	// +optional
	Vars []Variable `json:"vars,omitempty"`

	// +optional
	VarsFrom *VarsReference `json:"varsFrom,omitempty"`

	// The interval at which to reconcile the Terraform.
	// +required
	Interval metav1.Duration `json:"interval"`

	// A list of resources to be included in the health assessment.
	// +optional
	// HealthChecks []meta.NamespacedObjectKindReference `json:"healthChecks,omitempty"`

	// The interval at which to retry a previously failed reconciliation.
	// When not specified, the controller uses the TerraformSpec.Interval
	// value to retry failures.
	// +optional
	RetryInterval *metav1.Duration `json:"retryInterval,omitempty"`

	// Path to the directory containing Terraform (.tf) files.
	// Defaults to 'None', which translates to the root path of the SourceRef.
	// +optional
	Path string `json:"path,omitempty"`

	// The name of the Kubernetes service account to impersonate
	// when reconciling this Kustomization.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// Reference of the source where the kustomization file is.
	// +required
	SourceRef CrossNamespaceSourceReference `json:"sourceRef"`

	// This flag tells the controller to suspend subsequent kustomize executions,
	// it does not apply to already started executions. Defaults to false.
	// +optional
	Suspend bool `json:"suspend,omitempty"`

	// Timeout for validation, apply and health checking operations.
	// Defaults to 'Interval' duration.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// Force instructs the controller to recreate resources
	// when patching fails due to an immutable field change.
	// +kubebuilder:default:=false
	// +optional
	Force bool `json:"force,omitempty"`

	// Wait instructs the controller to check the health of all the reconciled resources.
	// When enabled, the HealthChecks are ignored. Defaults to false.
	// +kubebuilder:default:=false
	// +optional
	Wait bool `json:"wait,omitempty"`


	// A list of target secrets for the outputs to be written as.
	// +optional
	WriteOutputsToSecret *WriteOutputsToSecretSpec `json:"writeOutputsToSecret,omitempty"`
}

type PlanStatus struct {
	// +optional
	LastApplied string `json:"lastApplied,omitempty"`

	// +optional
	Pending string `json:"pending,omitempty"`
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

	// +optional
	AvailableOutputs []string `json:"availableOutputs,omitempty"`

	// +optional
	Plan PlanStatus `json:"plan,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Terraform is the Schema for the terraforms API
type Terraform struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TerraformSpec   `json:"spec,omitempty"`
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
	// +optional
	SecretSuffix string `json:"secretSuffix"`

	// +optional
	InClusterConfig bool `json:"inClusterConfig"`

	// +optional
	ConfigPath string `json:"configPath,omitempty"`

	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

const (
	TerraformKind             = "Terraform"
	TerraformFinalizer        = "finalizers.tf.contrib.fluxcd.io"
	MaxConditionMessageLength = 20000
	DisabledValue             = "disabled"
	HealthyCondition          = "Healthy"

	// ArtifactFailedReason represents the fact that the
	// source artifact download failed.
	ArtifactFailedReason = "ArtifactFailed"

	TFExecInstallFailedReason = "TFExecInstallFailed"
	TFExecNewFailedReason     = "TFExecNewFailed"
	TFExecInitFailedReason    = "TFExecInitFailed"
	TFExecPlanFailedReason    = "TFExecPlanFailed"
	TFExecApplyFailedReason   = "TFExecApplyFailed"
)

// SetKustomizationReadiness sets the ReadyCondition, ObservedGeneration, and LastAttemptedRevision, on the Kustomization.
func SetTerraformInit(terraform *Terraform, status metav1.ConditionStatus, reason, message string, revision string) {
	meta.SetResourceCondition(terraform, "Initialization", status, reason, trimString(message, MaxConditionMessageLength))
	terraform.Status.ObservedGeneration = terraform.Generation
	terraform.Status.LastAttemptedRevision = revision
}

// SetKustomizationReadiness sets the ReadyCondition, ObservedGeneration, and LastAttemptedRevision, on the Kustomization.
func SetTerraformApply(terraform *Terraform, status metav1.ConditionStatus, reason, message string, revision string) {
	meta.SetResourceCondition(terraform, "Apply", status, reason, trimString(message, MaxConditionMessageLength))
	terraform.Status.ObservedGeneration = terraform.Generation
	terraform.Status.LastAttemptedRevision = revision
}

// SetKustomizationReadiness sets the ReadyCondition, ObservedGeneration, and LastAttemptedRevision, on the Kustomization.
func SetTerraformReadiness(terraform *Terraform, status metav1.ConditionStatus, reason, message string, revision string) {
	meta.SetResourceCondition(terraform, meta.ReadyCondition, status, reason, trimString(message, MaxConditionMessageLength))
	terraform.Status.ObservedGeneration = terraform.Generation
	terraform.Status.LastAttemptedRevision = revision
}

func TerraformApplying(terraform Terraform, message string) Terraform {
	meta.SetResourceCondition(&terraform, "Apply", metav1.ConditionUnknown, meta.ProgressingReason, message)
	return terraform
}

func TerraformOutputAvailable(terraform Terraform, availableOutputs []string, message string) Terraform {
	meta.SetResourceCondition(&terraform, "Output", metav1.ConditionTrue, "TerraformOutputAvailable", message)
	(&terraform).Status.AvailableOutputs = availableOutputs
	return terraform
}

func TerraformApplied(terraform Terraform, message string) Terraform {
	meta.SetResourceCondition(&terraform, "Apply", metav1.ConditionTrue, "TerraformAppliedSucceed", message)
	plan := terraform.Status.Plan.Pending
	(&terraform).Status.Plan = PlanStatus{
		LastApplied: plan,
		Pending:     "",
	}
	return terraform
}

func TerraformPlannedWithChanges(terraform Terraform, planRev string, message string) Terraform {
	meta.SetResourceCondition(&terraform, "Plan", metav1.ConditionTrue, "TerraformPlannedSucceed", message)
	(&terraform).Status.Plan = PlanStatus{
		LastApplied: terraform.Status.Plan.LastApplied,
		Pending:     fmt.Sprintf("plan-%s", planRev),
	}
	return terraform
}

func TerraformPlannedNoChanges(terraform Terraform, message string) Terraform {
	meta.SetResourceCondition(&terraform, "Plan", metav1.ConditionFalse, "TerraformPlannedSucceed", message)
	(&terraform).Status.Plan = PlanStatus{
		LastApplied: terraform.Status.Plan.LastApplied,
		Pending:     "",
	}
	return terraform
}

// TerraformProgressing resets the conditions of the given Kustomization to a single
// ReadyCondition with status ConditionUnknown.
func TerraformProgressing(terraform Terraform, message string) Terraform {
	meta.SetResourceCondition(&terraform, meta.ReadyCondition, metav1.ConditionUnknown, meta.ProgressingReason, message)
	return terraform
}

// TerraformNotReady registers a failed apply attempt of the given Kustomization.
func TerraformNotReady(terraform Terraform, revision, reason, message string) Terraform {
	SetTerraformReadiness(&terraform, metav1.ConditionFalse, reason, trimString(message, MaxConditionMessageLength), revision)
	if revision != "" {
		terraform.Status.LastAttemptedRevision = revision
	}
	return terraform
}

// GetTimeout returns the timeout with default.
func (in Terraform) GetTimeout() time.Duration {
	duration := in.Spec.Interval.Duration - 30*time.Second
	if in.Spec.Timeout != nil {
		duration = in.Spec.Timeout.Duration
	}
	if duration < 30*time.Second {
		return 30 * time.Second
	}
	return duration
}

// GetRetryInterval returns the retry interval
func (in Terraform) GetRetryInterval() time.Duration {
	if in.Spec.RetryInterval != nil {
		return in.Spec.RetryInterval.Duration
	}
	return in.Spec.Interval.Duration
}

// GetStatusConditions returns a pointer to the Status.Conditions slice.
func (in *Terraform) GetStatusConditions() *[]metav1.Condition {
	return &in.Status.Conditions
}

func trimString(str string, limit int) string {
	if len(str) <= limit {
		return str
	}

	return str[0:limit] + "..."
}

func init() {
	SchemeBuilder.Register(&Terraform{}, &TerraformList{})
}
