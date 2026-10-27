package controllers

import (
	"github.com/flux-iac/tofu-controller/api/plan"
	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"
	"github.com/fluxcd/pkg/apis/meta"
)

// planCompression is the encoding api/plan applies to a plan body before
// storing it.
const planCompression = "gzip"

// Every mutation of status.artifactRefs lives in this file. Keeping the
// setters together means a new artifact is added in one place, and that the
// "set on success / clear when gone" pairing stays visible rather than being
// spread across the reconcile phases.

// ensureArtifactRefs returns the artifact reference block, creating it on
// first use.
func ensureArtifactRefs(terraform *infrav1.Terraform) *infrav1.ArtifactReferences {
	if terraform.Status.ArtifactRefs == nil {
		terraform.Status.ArtifactRefs = &infrav1.ArtifactReferences{}
	}

	return terraform.Status.ArtifactRefs
}

// setPlanArtifactRef publishes the plan artifact from the objects the runner
// reported writing. The first entry is the primary (chunk 0) and the count is
// the number of chunks.
//
// The runner reports only the canonical gzipped plan Secrets; the readable
// copies written for spec.storeReadablePlan are a separate artifact and are
// deliberately absent, so the chunk count stays truthful.
func setPlanArtifactRef(terraform *infrav1.Terraform, saved []*runner.SavedObject, planID string) {
	if len(saved) == 0 {
		clearPlanArtifactRef(terraform)
		return
	}

	primary := saved[0]

	ensureArtifactRefs(terraform).Plan = &infrav1.PlanArtifact{
		Locator: infrav1.ArtifactLocator{
			Type: infrav1.LocatorKubernetes,
			Kubernetes: &infrav1.KubernetesLocator{
				NamespacedObjectKindReference: objectReference(primary),
				Key:                           plan.TFPlanName,
			},
		},
		PlanID:      planID,
		Chunks:      int32(len(saved)),
		Compression: planCompression,
	}
}

// clearPlanArtifactRef drops the plan reference when no plan is persisted — a
// no-changes plan, or a destroy with an empty diff.
func clearPlanArtifactRef(terraform *infrav1.Terraform) {
	if terraform.Status.ArtifactRefs == nil {
		return
	}

	terraform.Status.ArtifactRefs.Plan = nil
}

// setStateArtifactRef publishes where the configured backend keeps state. The
// locator comes from resolveBackend, the same resolution that produced the HCL
// the runner executed.
func setStateArtifactRef(terraform *infrav1.Terraform, locator infrav1.ArtifactLocator) {
	ensureArtifactRefs(terraform).State = &infrav1.StateArtifact{Locator: locator}
}

// setOutputsArtifactRef publishes the outputs Secret after a successful
// WriteOutputs. The Secret is always in the Terraform's own namespace, because
// spec.writeOutputsToSecret.name is a same-namespace reference.
//
// No key is recorded: the Secret holds one entry per Terraform output, and
// status.availableOutputs already lists their names.
func setOutputsArtifactRef(terraform *infrav1.Terraform, secretName string) {
	ensureArtifactRefs(terraform).Outputs = &infrav1.OutputsArtifact{
		Locator: infrav1.SecretLocator(terraform.Namespace, secretName, ""),
	}
}

// clearOutputsArtifactRef drops the outputs reference on destroy, or when the
// configuration produced no outputs.
func clearOutputsArtifactRef(terraform *infrav1.Terraform) {
	if terraform.Status.ArtifactRefs == nil {
		return
	}

	terraform.Status.ArtifactRefs.Outputs = nil
}

// objectReference converts an object the runner reported writing into a Flux
// typed reference.
func objectReference(saved *runner.SavedObject) meta.NamespacedObjectKindReference {
	return meta.NamespacedObjectKindReference{
		APIVersion: saved.ApiVersion,
		Kind:       saved.Kind,
		Namespace:  saved.Namespace,
		Name:       saved.Name,
	}
}
