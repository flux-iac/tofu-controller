package controllers

import (
	"testing"

	. "github.com/onsi/gomega"
)

// crLabels simulates the metadata.labels a real Terraform CR carries in production:
// Flux kustomize labels, Helm toolkit labels, and arbitrary user labels.
// None of these must appear in the backend HCL labels block.
var crLabels = map[string]string{
	"kustomize.toolkit.fluxcd.io/name":      "global-stack-deploy",
	"kustomize.toolkit.fluxcd.io/namespace": "flux-system",
	"helm.toolkit.fluxcd.io/name":           "global-operator-helmrelease",
	"helm.toolkit.fluxcd.io/namespace":      "flux-space",
	"app.kubernetes.io/managed-by":          "Helm",
	"app.kubernetes.io/part-of":             "my-app",
	"engr.os.com/ring":                      "dev",
	"environment":                           "production",
}

// Test A — default path (no BackendConfig) and explicit BackendConfig with no Labels set:
// both call getLabelsAsHCL(nil) and must produce an empty HCL labels block.
// The backend writes tfstate, tfstateSecretSuffix, tfstateWorkspace, and
// app.kubernetes.io/managed-by natively onto every Secret; the HCL block must not
// inject any CR metadata labels.
func Test_000362_nil_labels_produces_empty_hcl_block(t *testing.T) {
	Spec("nil labels (default path or BackendConfig without Labels) must produce an empty HCL labels block.")
	g := NewWithT(t)

	hcl := getLabelsAsHCL(nil, 6)

	g.Expect(hcl).To(BeEmpty())

	for k, v := range crLabels {
		g.Expect(hcl).NotTo(ContainSubstring(k), "CR label key %q must not appear in HCL", k)
		g.Expect(hcl).NotTo(ContainSubstring(v), "CR label value %q must not appear in HCL", v)
	}
}

// Test B — explicit BackendConfig with Labels set: HCL contains exactly those labels
// and none of the CR metadata labels.
func Test_000362_backend_config_labels_used_verbatim(t *testing.T) {
	Spec("BackendConfig.Labels must appear verbatim in HCL; CR metadata labels must not.")
	g := NewWithT(t)

	backendLabels := map[string]string{
		"app": "my-service",
		"env": "staging",
	}

	hcl := getLabelsAsHCL(backendLabels, 6)

	g.Expect(hcl).To(ContainSubstring(`"app"`))
	g.Expect(hcl).To(ContainSubstring(`"my-service"`))
	g.Expect(hcl).To(ContainSubstring(`"env"`))
	g.Expect(hcl).To(ContainSubstring(`"staging"`))

	for k, v := range crLabels {
		g.Expect(hcl).NotTo(ContainSubstring(k), "CR label key %q must not appear in HCL", k)
		g.Expect(hcl).NotTo(ContainSubstring(v), "CR label value %q must not appear in HCL", v)
	}
}
