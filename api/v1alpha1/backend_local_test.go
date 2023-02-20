package v1alpha1

import (
	. "github.com/onsi/gomega"
	"testing"
)

func TestLocalBackend_ToHCL(t *testing.T) {
	g := NewGomegaWithT(t)

	testCases := []struct {
		Name         string
		LocalBackend *LocalBackend
		ExpectedHCL  string
	}{
		{
			Name: "Test case with both path and workspace_dir specified",
			LocalBackend: &LocalBackend{
				Path:         "/path/to/tfstate",
				WorkspaceDir: "/path/to/workspaces",
			},
			ExpectedHCL: "terraform {\n  backend \"local\" {\n    path = \"/path/to/tfstate\"\n    workspace_dir = \"/path/to/workspaces\"\n  }\n}\n",
		},
		{
			Name: "Test case with only path specified",
			LocalBackend: &LocalBackend{
				Path: "/path/to/tfstate",
			},
			ExpectedHCL: "terraform {\n  backend \"local\" {\n    path = \"/path/to/tfstate\"\n  }\n}\n",
		},
		{
			Name: "Test case with only workspace_dir specified",
			LocalBackend: &LocalBackend{
				WorkspaceDir: "/path/to/workspaces",
			},
			ExpectedHCL: "terraform {\n  backend \"local\" {\n    workspace_dir = \"/path/to/workspaces\"\n  }\n}\n",
		},
		{
			Name:         "Test case with neither path nor workspace_dir specified",
			LocalBackend: &LocalBackend{},
			ExpectedHCL:  "terraform {\n  backend \"local\" {\n  }\n}\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			hcl, err := tc.LocalBackend.ToHCL()
			g.Expect(err).To(BeNil())
			g.Expect(hcl).To(Equal(tc.ExpectedHCL))
		})
	}
}
