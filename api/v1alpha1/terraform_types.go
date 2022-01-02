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
	"strings"
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
	// Name is the name of the Secret to be written
	// +required
	Name string `json:"name"`

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

	// Destroy produces a destroy plan. Applying the plan will destroy all resources.
	// +optional
	Destroy bool `json:"destroy,omitempty"`

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

	// The interval at which to retry a previously failed reconciliation.
	// When not specified, the controller uses the TerraformSpec.Interval
	// value to retry failures.
	// +optional
	RetryInterval *metav1.Duration `json:"retryInterval,omitempty"`

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

	// A list of target secrets for the outputs to be written as.
	// +optional
	WriteOutputsToSecret *WriteOutputsToSecretSpec `json:"writeOutputsToSecret,omitempty"`

	// +optional
	// PushSpec *PushSpec `json:"pushSpec,omitempty"`
}

type PlanStatus struct {
	// +optional
	LastApplied string `json:"lastApplied,omitempty"`

	// +optional
	Pending string `json:"pending,omitempty"`

	// +optional
	IsDestroyPlan bool `json:"isDestroyPlan,omitempty"`
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

	// ArtifactFailedReason represents the fact that the
	// source artifact download failed.
	ArtifactFailedReason = "ArtifactFailed"

	TFExecNewFailedReason      = "TFExecNewFailed"
	TFExecInitFailedReason     = "TFExecInitFailed"
	TFExecPlanFailedReason     = "TFExecPlanFailed"
	TFExecApplyFailedReason    = "TFExecApplyFailed"
	TFExecOutputFailedReason   = "TFExecOutputFailed"
	OutputsWritingFailedReason = "OutputsWritingFailed"
)

// SetTerraformReadiness sets the ReadyCondition, ObservedGeneration, and LastAttemptedRevision, on the Terraform.
func SetTerraformReadiness(terraform *Terraform, status metav1.ConditionStatus, reason, message string, revision string) {
	meta.SetResourceCondition(terraform, meta.ReadyCondition, status, reason, trimString(message, MaxConditionMessageLength))
	terraform.Status.ObservedGeneration = terraform.Generation
	terraform.Status.LastAttemptedRevision = revision
}

func TerraformApplying(terraform Terraform, revision string, message string) Terraform {
	meta.SetResourceCondition(&terraform, "Apply", metav1.ConditionUnknown, meta.ProgressingReason, message)
	if revision != "" {
		(&terraform).Status.LastAppliedRevision = revision
	}
	return terraform
}

func TerraformOutputsAvailable(terraform Terraform, availableOutputs []string, message string) Terraform {
	meta.SetResourceCondition(&terraform, "Output", metav1.ConditionTrue, "TerraformOutputsAvailable", message)
	(&terraform).Status.AvailableOutputs = availableOutputs
	return terraform
}

func TerraformOutputsWritten(terraform Terraform, revision string, message string) Terraform {
	meta.SetResourceCondition(&terraform, "Output", metav1.ConditionTrue, "TerraformOutputsWritten", message)

	SetTerraformReadiness(&terraform, metav1.ConditionTrue, "TerraformOutputsWritten", message, revision)
	return terraform
}

func TerraformApplied(terraform Terraform, revision string, message string) Terraform {
	meta.SetResourceCondition(&terraform, "Apply", metav1.ConditionTrue, "TerraformAppliedSucceed", message)
	(&terraform).Status.Plan = PlanStatus{
		LastApplied:   terraform.Status.Plan.Pending,
		Pending:       "",
		IsDestroyPlan: terraform.Status.Plan.IsDestroyPlan,
	}
	if revision != "" {
		(&terraform).Status.LastAppliedRevision = revision
	}
	return terraform
}

func TerraformPlannedWithChanges(terraform Terraform, revision string, message string) Terraform {
	planRev := strings.Replace(revision, "/", "-", 1)
	meta.SetResourceCondition(&terraform, "Plan", metav1.ConditionTrue, "TerraformPlannedWithChanges", message)
	(&terraform).Status.Plan = PlanStatus{
		LastApplied:   terraform.Status.Plan.LastApplied,
		Pending:       fmt.Sprintf("plan-%s", planRev),
		IsDestroyPlan: terraform.Spec.Destroy,
	}
	if revision != "" {
		(&terraform).Status.LastAttemptedRevision = revision
	}

	SetTerraformReadiness(&terraform, metav1.ConditionUnknown, "TerraformPlannedWithChanges", message, revision)
	return terraform
}

func TerraformPlannedNoChanges(terraform Terraform, revision string, message string) Terraform {
	meta.SetResourceCondition(&terraform, "Plan", metav1.ConditionFalse, "TerraformPlannedNoChanges", message)
	(&terraform).Status.Plan = PlanStatus{
		LastApplied:   terraform.Status.Plan.LastApplied,
		Pending:       "",
		IsDestroyPlan: terraform.Spec.Destroy,
	}
	if revision != "" {
		(&terraform).Status.LastAttemptedRevision = revision
	}

	SetTerraformReadiness(&terraform, metav1.ConditionTrue, "TerraformPlannedNoChanges", message, revision)
	return terraform
}

// TerraformProgressing resets the conditions of the given Terraform to a single
// ReadyCondition with status ConditionUnknown.
func TerraformProgressing(terraform Terraform, message string) Terraform {
	meta.SetResourceCondition(&terraform, meta.ReadyCondition, metav1.ConditionUnknown, meta.ProgressingReason, message)
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
