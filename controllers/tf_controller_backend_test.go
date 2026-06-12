package controllers

import (
	"testing"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetBackendLabels(t *testing.T) {
	crLabels := map[string]string{
		"app.kubernetes.io/managed-by":          "terraform",
		"tfstate":                               "true",
		"kustomize.toolkit.fluxcd.io/name":      "infra",
		"kustomize.toolkit.fluxcd.io/namespace": "flux-system",
	}

	tests := []struct {
		name     string
		tf       *infrav1.Terraform
		expected map[string]string
	}{
		{
			name: "BackendConfig is nil → falls back to terraform.Labels",
			tf: &infrav1.Terraform{
				ObjectMeta: metav1.ObjectMeta{Labels: crLabels},
				Spec:       infrav1.TerraformSpec{BackendConfig: nil},
			},
			expected: crLabels,
		},
		{
			name: "BackendConfig.Labels is nil → falls back to terraform.Labels",
			tf: &infrav1.Terraform{
				ObjectMeta: metav1.ObjectMeta{Labels: crLabels},
				Spec: infrav1.TerraformSpec{
					BackendConfig: &infrav1.BackendConfigSpec{Labels: nil},
				},
			},
			expected: crLabels,
		},
		{
			name: "BackendConfig.Labels is set → uses exactly those, ignoring terraform.Labels",
			tf: &infrav1.Terraform{
				ObjectMeta: metav1.ObjectMeta{Labels: crLabels},
				Spec: infrav1.TerraformSpec{
					BackendConfig: &infrav1.BackendConfigSpec{
						Labels: map[string]string{
							"tfstate":             "true",
							"tfstateWorkspace":    "default",
							"tfstateSecretSuffix": "my-tf",
						},
					},
				},
			},
			expected: map[string]string{
				"tfstate":             "true",
				"tfstateWorkspace":    "default",
				"tfstateSecretSuffix": "my-tf",
			},
		},
		{
			name: "BackendConfig.Labels is empty non-nil → explicit opt-out, no labels",
			tf: &infrav1.Terraform{
				ObjectMeta: metav1.ObjectMeta{Labels: crLabels},
				Spec: infrav1.TerraformSpec{
					BackendConfig: &infrav1.BackendConfigSpec{
						Labels: map[string]string{},
					},
				},
			},
			expected: map[string]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(getBackendLabels(tc.tf)).To(Equal(tc.expected))
		})
	}
}
