package controllers

import (
	"fmt"
	"strings"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

// ParseRunnerTopologySpreadConstraints parses the controller-wide default
// topologySpreadConstraints for runner pods. The input is a JSON or YAML list
// of Kubernetes TopologySpreadConstraint objects. Empty input returns nil.
//
// Unknown fields are rejected and each constraint is validated, so a typo in
// the flag fails at startup instead of when the first runner pod is created.
func ParseRunnerTopologySpreadConstraints(s string) ([]corev1.TopologySpreadConstraint, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}

	var constraints []corev1.TopologySpreadConstraint
	if err := yaml.UnmarshalStrict([]byte(s), &constraints); err != nil {
		return nil, fmt.Errorf("failed to parse topology spread constraints: %w", err)
	}

	for i := range constraints {
		if err := validateTopologySpreadConstraint(&constraints[i]); err != nil {
			return nil, fmt.Errorf("constraint %d: %w", i, err)
		}
	}

	return constraints, nil
}

// runnerTopologySpreadConstraints returns the constraints for a runner pod: the
// Terraform resource's own list when set, otherwise a copy of the controller-wide
// default, otherwise nil. The two are never merged.
func (r *TerraformReconciler) runnerTopologySpreadConstraints(terraform *infrav1.Terraform) []corev1.TopologySpreadConstraint {
	if constraints := terraform.Spec.RunnerPodTemplate.Spec.TopologySpreadConstraints; len(constraints) > 0 {
		return constraints
	}

	var constraints []corev1.TopologySpreadConstraint
	for i := range r.RunnerTopologySpreadConstraints {
		constraints = append(constraints, *r.RunnerTopologySpreadConstraints[i].DeepCopy())
	}
	return constraints
}

func validateTopologySpreadConstraint(c *corev1.TopologySpreadConstraint) error {
	if c.TopologyKey == "" {
		return fmt.Errorf("topologyKey must not be empty")
	}
	if c.MaxSkew < 1 {
		return fmt.Errorf("maxSkew must be at least 1, got %d", c.MaxSkew)
	}
	switch c.WhenUnsatisfiable {
	case corev1.DoNotSchedule, corev1.ScheduleAnyway:
	default:
		return fmt.Errorf("whenUnsatisfiable must be %q or %q, got %q",
			corev1.DoNotSchedule, corev1.ScheduleAnyway, c.WhenUnsatisfiable)
	}
	if c.MinDomains != nil && c.WhenUnsatisfiable != corev1.DoNotSchedule {
		return fmt.Errorf("minDomains can only be set when whenUnsatisfiable is %q", corev1.DoNotSchedule)
	}
	return nil
}
