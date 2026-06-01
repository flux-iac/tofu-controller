package runner

import (
	"testing"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
)

// countByType tallies tfexec plan options by their concrete type name so we can
// assert which options appendPlanSpecOptions produced. The option structs keep
// their fields unexported, so the concrete value (e.g. -lock=false vs
// -lock=true) is verified end-to-end via the runner logs rather than here.
func countByType(opts []tfexec.PlanOption) map[string]int {
	counts := map[string]int{}
	for _, opt := range opts {
		switch opt.(type) {
		case *tfexec.LockOption:
			counts["lock"]++
		case *tfexec.RefreshOnlyOption:
			counts["refreshOnly"]++
		case *tfexec.ReplaceOption:
			counts["replace"]++
		case *tfexec.ParallelismOption:
			counts["parallelism"]++
		}
	}
	return counts
}

func TestAppendPlanSpecOptions(t *testing.T) {
	tests := []struct {
		name     string
		plan     *infrav1.PlanSpec
		expected map[string]int
	}{
		{
			name:     "nil spec adds nothing",
			plan:     nil,
			expected: map[string]int{},
		},
		{
			name:     "empty spec adds nothing",
			plan:     &infrav1.PlanSpec{},
			expected: map[string]int{},
		},
		{
			name:     "lock disabled adds a single lock option",
			plan:     &infrav1.PlanSpec{Lock: ptr.To(false)},
			expected: map[string]int{"lock": 1},
		},
		{
			name:     "lock enabled adds a single lock option",
			plan:     &infrav1.PlanSpec{Lock: ptr.To(true)},
			expected: map[string]int{"lock": 1},
		},
		{
			name:     "refreshOnly enabled adds a refresh-only option",
			plan:     &infrav1.PlanSpec{RefreshOnly: true},
			expected: map[string]int{"refreshOnly": 1},
		},
		{
			name:     "refreshOnly disabled adds nothing",
			plan:     &infrav1.PlanSpec{RefreshOnly: false},
			expected: map[string]int{},
		},
		{
			name:     "replace adds one option per address",
			plan:     &infrav1.PlanSpec{Replace: []string{"aws_instance.a", "aws_instance.b"}},
			expected: map[string]int{"replace": 2},
		},
		{
			name:     "positive parallelism adds a parallelism option",
			plan:     &infrav1.PlanSpec{Parallelism: 20},
			expected: map[string]int{"parallelism": 1},
		},
		{
			name:     "zero parallelism adds nothing",
			plan:     &infrav1.PlanSpec{Parallelism: 0},
			expected: map[string]int{},
		},
		{
			name: "all options combined",
			plan: &infrav1.PlanSpec{
				Lock:        ptr.To(false),
				RefreshOnly: true,
				Replace:     []string{"aws_instance.a"},
				Parallelism: 5,
			},
			expected: map[string]int{"lock": 1, "refreshOnly": 1, "replace": 1, "parallelism": 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := appendPlanSpecOptions(nil, tt.plan)
			assert.Equal(t, tt.expected, countByType(opts))
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
