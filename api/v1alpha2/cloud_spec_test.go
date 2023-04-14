package v1alpha2

import (
	. "github.com/onsi/gomega"
	"strings"
	"testing"
)

// CloudSpec defines the desired state of Terraform Cloud
func TestCloudSpec(t *testing.T) {
	g := NewGomegaWithT(t)
	cloudSpec := &CloudSpec{
		Organization: "test-org",
		Workspaces: &CloudWorkspacesSpec{
			Name: "dev",
			Tags: []string{"test-tag", "test-tag-2"},
		},
		Hostname: "app.terraform.io",
		Token:    "test-token",
	}

	fixture := strings.TrimLeft(`
terraform {
  cloud {
    organization = "test-org"
    workspaces {
      name = "dev"
      tags = ["test-tag", "test-tag-2"]
    }
    hostname = "app.terraform.io"
    token = "test-token"
  }
}
`, "\n")
	g.Expect(cloudSpec.ToHCL()).To(Equal(fixture))
}
