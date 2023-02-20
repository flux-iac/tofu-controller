package v1alpha1

import (
	"fmt"
	. "github.com/onsi/gomega"
	"testing"
)

func TestRemoteBackend_ToHCL(t *testing.T) {
	tests := []struct {
		backend  *RemoteBackend
		expected string
	}{
		{
			backend: &RemoteBackend{
				Hostname:     "app.terraform.io",
				Organization: "myorg",
				Token:        "",
				Workspaces: &RemoteBackendWorkspaces{
					Name:   "",
					Prefix: "prod-",
				},
			},
			expected: "terraform {\n  backend \"remote\" {\n    hostname = \"app.terraform.io\"\n    organization = \"myorg\"\n    workspaces {\n      prefix = \"prod-\"\n    }\n  }\n}\n",
		},
		{
			backend: &RemoteBackend{
				Hostname:     "",
				Organization: "myorg",
				Token:        "token",
				Workspaces: &RemoteBackendWorkspaces{
					Name:   "prod",
					Prefix: "",
				},
			},
			expected: "terraform {\n  backend \"remote\" {\n    organization = \"myorg\"\n    token = \"token\"\n    workspaces {\n      name = \"prod\"\n    }\n  }\n}\n",
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("Test %d", i), func(t *testing.T) {
			g := NewGomegaWithT(t)
			hcl, err := tt.backend.ToHCL()
			g.Expect(err).To(BeNil())
			g.Expect(hcl).To(Equal(tt.expected))
		})
	}
}

func TestRemoteBackend_ToHCL_Fail(t *testing.T) {
	g := NewGomegaWithT(t)
	b := &RemoteBackend{}
	_, err := b.ToHCL()
	g.Expect(err).To(MatchError("workspaces configuration is required for RemoteBackend"))

	b.Workspaces = &RemoteBackendWorkspaces{Name: "", Prefix: ""}
	_, err = b.ToHCL()
	g.Expect(err).To(MatchError("either name or prefix is required in Workspaces configuration"))

	b.Workspaces.Name = "test-workspace"
	b.Workspaces.Prefix = "test-workspace"
	_, err = b.ToHCL()
	g.Expect(err).To(MatchError("only one of name or prefix can be set in Workspaces configuration"))
}
