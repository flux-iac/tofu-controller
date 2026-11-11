package controllers

import (
	"testing"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// +kubebuilder:docs-gen:collapse=Imports

func Test_000363_runner_topology_spread_test(t *testing.T) {
	Spec("This spec describes how topologySpreadConstraints reach the runner pod.")

	newConstraint := func(maxSkew int32) corev1.TopologySpreadConstraint {
		return corev1.TopologySpreadConstraint{
			MaxSkew:            maxSkew,
			MinDomains:         ptr.To(int32(100)),
			TopologyKey:        "kubernetes.io/hostname",
			WhenUnsatisfiable:  corev1.DoNotSchedule,
			NodeAffinityPolicy: ptr.To(corev1.NodeInclusionPolicyHonor),
			NodeTaintsPolicy:   ptr.To(corev1.NodeInclusionPolicyHonor),
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/created-by": "tofu-controller"},
			},
		}
	}

	newTerraform := func(constraints []corev1.TopologySpreadConstraint) *infrav1.Terraform {
		return &infrav1.Terraform{
			ObjectMeta: metav1.ObjectMeta{Name: "topology-spread-test", Namespace: "flux-system"},
			Spec: infrav1.TerraformSpec{
				RunnerPodTemplate: infrav1.RunnerPodTemplate{
					Spec: infrav1.RunnerPodSpec{TopologySpreadConstraints: constraints},
				},
			},
		}
	}

	newReconciler := func() *TerraformReconciler {
		return &TerraformReconciler{RunnerGRPCPort: 30000}
	}

	t.Run("CR value is copied into the runner pod", func(t *testing.T) {
		g := NewWithT(t)
		Given("a Terraform resource with topologySpreadConstraints")
		crValue := []corev1.TopologySpreadConstraint{newConstraint(10)}

		spec := newReconciler().runnerPodSpec(newTerraform(crValue), "runner.tls-123")

		By("checking the runner pod carries the CR value")
		g.Expect(spec.TopologySpreadConstraints).To(Equal(crValue))
	})

	t.Run("no constraints are set when nothing is configured", func(t *testing.T) {
		g := NewWithT(t)
		Given("a Terraform resource without constraints")

		spec := newReconciler().runnerPodSpec(newTerraform(nil), "runner.tls-123")

		By("checking the runner pod has no constraints")
		g.Expect(spec.TopologySpreadConstraints).To(BeNil())
	})
}
