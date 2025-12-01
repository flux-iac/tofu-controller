package runner

import (
	"testing"

	"github.com/flux-iac/tofu-controller/api/planid"
	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	. "github.com/onsi/gomega"
)

func TestSaveTFPlanWithMultipleChunks(t *testing.T) {
	g := NewGomegaWithT(t)

	terraform := &infrav1.Terraform{}

	server := &TerraformRunnerServer{
		InstanceID: "51b32416-d76d-4720-b2ef-1c13996d3c4a",
		terraform:  terraform,
	}

	req := &SaveTFPlanRequest{
		Name:       "a-really-large-plan",
		Namespace:  "terraform",
		TfInstance: server.InstanceID,
		Revision:   "master@sha1:453f0eeb",
	}

	// create 4MB of plan data.
	// intentionally not gzipping as this will make the data a lot smaller
	planData := make([]byte, 4*1024*1024) // 4MB of plans!

	planId := planid.GetPlanID(req.Revision)

	secrets, err := server.generatePlanSecrets(req.Name, req.Namespace, planId, "", "plan-uuid-1", planData)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(len(secrets)).To(Equal(4), "plan data should have been chunked into four secrets")
}
