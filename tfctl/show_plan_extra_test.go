package tfctl

import (
	"testing"

	. "github.com/onsi/gomega"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
)

func TestPlanExists_TypedRefPreferred(t *testing.T) {
	g := NewWithT(t)
	tf := &infrav1.Terraform{
		Status: infrav1.TerraformStatus{
			ArtifactRefs: &infrav1.ArtifactReferences{
				Plan: &infrav1.PlanArtifact{
					Locator: infrav1.SecretLocator("shared", "tfplan-default-hello", "tfplan"),
				},
			},
		},
	}

	g.Expect(planExists(tf)).To(BeTrue())
	g.Expect(planRefNamespace(tf, "fallback-ns")).To(Equal("shared"))
}

func TestPlanExists_FallbackToLegacyField(t *testing.T) {
	g := NewWithT(t)
	tf := &infrav1.Terraform{}
	g.Expect(planExists(tf)).To(BeFalse())

	tf.Status.Plan.Pending = "plan-id-123"
	g.Expect(planExists(tf)).To(BeTrue())
	g.Expect(planRefNamespace(tf, "fallback-ns")).To(Equal("fallback-ns"))
}

// A plan that is not in this cluster must not hand the CLI a namespace it
// would then read from the wrong cluster.
func TestPlanRefNamespace_NonKubernetesLocatorFallsBack(t *testing.T) {
	g := NewWithT(t)
	tf := &infrav1.Terraform{
		Status: infrav1.TerraformStatus{
			ArtifactRefs: &infrav1.ArtifactReferences{
				Plan: &infrav1.PlanArtifact{
					Locator: infrav1.ArtifactLocator{
						Type: infrav1.LocatorExternal,
						External: &infrav1.ExternalLocator{
							Backend:  "s3",
							Location: "s3://bucket/plan",
						},
					},
				},
			},
		},
	}

	g.Expect(planExists(tf)).To(BeTrue())
	g.Expect(planRefNamespace(tf, "fallback-ns")).To(Equal("fallback-ns"))
}
