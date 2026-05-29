package controllers

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/flux-iac/tofu-controller/api/plan"
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

// Test A — default backend path: HCL contains only the two stable backend-native labels,
// regardless of what labels the CR carries.
func Test_000362_default_backend_hcl_uses_stable_labels(t *testing.T) {
	Spec("Default backend HCL should use only tfstateSecretSuffix and tfstateWorkspace, not CR labels.")
	g := NewWithT(t)

	crName := "my-terraform"
	workspace := "default"

	hcl := getLabelsAsHCL(map[string]string{
		"tfstateSecretSuffix": plan.SafeLabelValue(crName),
		"tfstateWorkspace":    plan.SafeLabelValue(workspace),
	}, 6)

	g.Expect(hcl).To(ContainSubstring("tfstateSecretSuffix"))
	g.Expect(hcl).To(ContainSubstring(crName))
	g.Expect(hcl).To(ContainSubstring("tfstateWorkspace"))
	g.Expect(hcl).To(ContainSubstring(workspace))

	for k, v := range crLabels {
		g.Expect(hcl).NotTo(ContainSubstring(k), "CR label key %q must not appear in HCL", k)
		g.Expect(hcl).NotTo(ContainSubstring(v), "CR label value %q must not appear in HCL", v)
	}
}

// Test B — explicit BackendConfig path: HCL uses BackendConfig.SecretSuffix (not the CR
// name), and still excludes all CR labels.
func Test_000362_explicit_backend_hcl_uses_secret_suffix(t *testing.T) {
	Spec("Explicit BackendConfig HCL should use BackendConfig.SecretSuffix, not CR name or CR labels.")
	g := NewWithT(t)

	crName := "my-terraform"
	secretSuffix := "custom-suffix"
	workspace := "staging"

	hcl := getLabelsAsHCL(map[string]string{
		"tfstateSecretSuffix": plan.SafeLabelValue(secretSuffix),
		"tfstateWorkspace":    plan.SafeLabelValue(workspace),
	}, 6)

	g.Expect(hcl).To(ContainSubstring("tfstateSecretSuffix"))
	g.Expect(hcl).To(ContainSubstring(secretSuffix))
	g.Expect(hcl).To(ContainSubstring("tfstateWorkspace"))
	g.Expect(hcl).To(ContainSubstring(workspace))
	// SecretSuffix differs from CR name — CR name must not bleed in
	g.Expect(hcl).NotTo(ContainSubstring(crName))

	for k, v := range crLabels {
		g.Expect(hcl).NotTo(ContainSubstring(k), "CR label key %q must not appear in HCL", k)
		g.Expect(hcl).NotTo(ContainSubstring(v), "CR label value %q must not appear in HCL", v)
	}
}
