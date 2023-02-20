package v1alpha1

import (
	. "github.com/onsi/gomega"
	"testing"
)

func TestAzureRMBackendToHCL(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		desc             string
		backend          AzureRMBackend
		expectedOutput   string
		expectedErrorMsg string
	}{
		{
			desc: "with access_key",
			backend: AzureRMBackend{
				StorageAccountName: "test-account",
				ContainerName:      "test-container",
				Key:                "test-key",
				AccessKey:          "test-access-key",
				UseMsi:             false,
			},
			expectedOutput: `terraform {
  backend "azurerm" {
    storage_account_name = "test-account"
    container_name = "test-container"
    key = "test-key"
    access_key = "test-access-key"
  }
}
`,
			expectedErrorMsg: "",
		},
		{
			desc: "with use_msi",
			backend: AzureRMBackend{
				StorageAccountName: "test-account",
				ContainerName:      "test-container",
				Key:                "test-key",
				UseMsi:             true,
			},
			expectedOutput: `terraform {
  backend "azurerm" {
    storage_account_name = "test-account"
    container_name = "test-container"
    use_msi = true
    resource_group_name = "test-resource-group"
    msi_endpoint = "http://"
    subscription_id = "test-subscription-id"
    tenant_id = "test-tenant-id"
  }
}
`,
			expectedErrorMsg: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			output, err := tt.backend.ToHCL()
			if tt.expectedErrorMsg != "" {
				g.Expect(err).To(MatchError(tt.expectedErrorMsg))
			} else {
				g.Expect(err).To(BeNil())
				g.Expect(output).To(Equal(tt.expectedOutput))
			}
		})
	}
}
