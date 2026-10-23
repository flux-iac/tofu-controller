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

// Action describes the stage of the reconcile loop an event reports on. It is
// sent as the action of notification-controller events. Kubernetes core/v1
// Events have no action field and do not record it.
type Action string

// String returns the string representation of the Action.
func (a Action) String() string {
	return string(a)
}

const (
	// ActionReconcile denotes the overall outcome of the reconcile loop.
	ActionReconcile Action = "Reconcile"

	// ActionResolveSource resolves the source reference and fetches its
	// artifact.
	ActionResolveSource Action = "ResolveSource"

	// ActionCheckDependencies verifies that every Terraform object referenced
	// by spec.dependsOn is ready before reconciliation proceeds.
	ActionCheckDependencies Action = "CheckDependencies"

	// ActionProvisionRunner creates the runner Pod for the Terraform object.
	ActionProvisionRunner Action = "ProvisionRunner"

	// ActionPlan runs terraform plan.
	ActionPlan Action = "Plan"

	// ActionApply runs terraform apply.
	ActionApply Action = "Apply"

	// ActionDestroy runs terraform destroy.
	ActionDestroy Action = "Destroy"

	// ActionDriftDetection checks the live infrastructure for drift from the
	// Terraform state.
	ActionDriftDetection Action = "DriftDetection"

	// ActionHealthCheck runs the health checks in spec.healthChecks.
	ActionHealthCheck Action = "HealthCheck"

	// ActionWriteOutputs writes the Terraform outputs to a Secret.
	ActionWriteOutputs Action = "WriteOutputs"
)
