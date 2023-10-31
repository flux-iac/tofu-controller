package controllers

import (
	"testing"

	. "github.com/onsi/gomega"
	infrav1 "github.com/weaveworks/tf-controller/api/v1alpha2"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000180_should_detect_drift_test(t *testing.T) {
	Spec("This spec describes behaviour of the `shouldDetectDrift`.")

	g := NewWithT(t)

	tf1 := infrav1.Terraform{
		Spec: infrav1.TerraformSpec{
			Destroy: true,
		},
	}
	It("should be false in the destroy mode.")
	g.Expect(reconciler.shouldDetectDrift(tf1, "main/1234")).Should(BeFalse())

	tf2 := infrav1.Terraform{
		Spec: infrav1.TerraformSpec{
			Destroy: false,
		},
	}
	It("should be false for the newly created object.")
	g.Expect(reconciler.shouldDetectDrift(tf2, "main/1234")).Should(BeFalse())

	tf3 := infrav1.Terraform{
		Spec: infrav1.TerraformSpec{
			Destroy: false,
		},
		Status: infrav1.TerraformStatus{
			LastAttemptedRevision: "main/1234",
			LastPlannedRevision:   "main/1234",
			LastAppliedRevision:   "main/1234",
			Plan: infrav1.PlanStatus{
				Pending: "",
			},
		},
	}
	It("should be true for a normally applied object.")
	g.Expect(reconciler.shouldDetectDrift(tf3, "main/1234")).Should(BeTrue())

	tf4 := infrav1.Terraform{
		Spec: infrav1.TerraformSpec{
			Destroy: false,
		},
		Status: infrav1.TerraformStatus{
			LastAttemptedRevision: "main/2345",
			LastPlannedRevision:   "main/2345",
			LastAppliedRevision:   "main/1234",
			Plan: infrav1.PlanStatus{
				Pending: "",
			},
		},
	}
	It("should be true for a non-TF source change.")
	g.Expect(reconciler.shouldDetectDrift(tf4, "main/2345")).Should(BeTrue())

	tf5 := infrav1.Terraform{
		Spec: infrav1.TerraformSpec{
			Destroy: false,
		},
		Status: infrav1.TerraformStatus{
			LastAttemptedRevision: "main/2345",
			LastPlannedRevision:   "main/2345",
			// no applied revision record because the old object was deleted and the new one re-created
			LastAppliedRevision: "",
			Plan: infrav1.PlanStatus{
				Pending: "",
			},
		},
	}
	It("should be true for a deleted / re-created object.")
	g.Expect(reconciler.shouldDetectDrift(tf5, "main/2345")).Should(BeTrue())

	tf5_1 := infrav1.Terraform{
		Spec: infrav1.TerraformSpec{
			Destroy: false,
		},
		Status: infrav1.TerraformStatus{
			LastAttemptedRevision: "main/2345",
			LastPlannedRevision:   "main/2345",
			// no applied revision record because the old object was deleted and the new one re-created
			LastAppliedRevision: "",
			Plan: infrav1.PlanStatus{
				Pending: "plan-main-1234",
			},
		},
	}
	It("should be false for an object with the pending plan.")
	g.Expect(reconciler.shouldDetectDrift(tf5_1, "main/2345")).Should(BeFalse())

	It("should be true for when ApprovePlan is disable")
	tf6 := infrav1.Terraform{
		Spec: infrav1.TerraformSpec{
			Destroy:     false,
			ApprovePlan: infrav1.ApprovePlanDisableValue,
		},
		Status: infrav1.TerraformStatus{
			LastAttemptedRevision: "main/2345",
			LastPlannedRevision:   "main/2345",
			LastAppliedRevision:   "main/1234",
		},
	}

	g.Expect(reconciler.shouldDetectDrift(tf6, "main/1234")).Should(BeTrue())
}
