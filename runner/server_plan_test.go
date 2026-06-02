package runner

import (
	"testing"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
)

// TestAppendPlanSpecOptions asserts on the exact tfexec option values, not just
// their types: tfexec.Lock(false) and tfexec.Lock(true) compare unequal under
// assert.Equal (deep comparison of the option struct), so this distinguishes
// -lock=false from -lock=true rather than merely confirming a lock option exists.
func TestAppendPlanSpecOptions(t *testing.T) {
	tests := []struct {
		name     string
		plan     *infrav1.PlanSpec
		expected []tfexec.PlanOption
	}{
		{
			name:     "nil spec adds nothing",
			plan:     nil,
			expected: nil,
		},
		{
			name:     "empty spec adds nothing",
			plan:     &infrav1.PlanSpec{},
			expected: nil,
		},
		{
			name:     "lock disabled emits -lock=false",
			plan:     &infrav1.PlanSpec{Lock: ptr.To(false)},
			expected: []tfexec.PlanOption{tfexec.Lock(false)},
		},
		{
			name:     "lock enabled emits -lock=true",
			plan:     &infrav1.PlanSpec{Lock: ptr.To(true)},
			expected: []tfexec.PlanOption{tfexec.Lock(true)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := appendPlanSpecOptions(nil, tt.plan)
			assert.Equal(t, tt.expected, opts)
		})
	}
}

func TestSanitizeLog(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no_sanitization_needed",
			input:    `This is a test log.`,
			expected: `This is a test log.`,
		},
		{
			name: "sanitization_needed",
			input: `on generated.auto.tfvars.json line 1:
			1: {"secret_subject":"test-","subject":"test"}`,
			expected: `on generated.auto.tfvars.json line 1:
			1: {"secret_subject":"***","subject":"***"}`,
		},
		{
			name: "sanitization_needed_multiple_lines",
			input: `This is a test log.
			on generated.auto.tfvars.json line 1:
			1: {"secret_subject":"test-","subject":"test"}
			This is another test log.
			on generated.auto.tfvars.json line 2:
			2: {"secret_subject":"test-","subject":"test"}`,
			expected: `This is a test log.
			on generated.auto.tfvars.json line 1:
			1: {"secret_subject":"***","subject":"***"}
			This is another test log.
			on generated.auto.tfvars.json line 2:
			2: {"secret_subject":"***","subject":"***"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeLog(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeLog() = %v, want %v", result, tt.expected)
			}
		})
	}
}
