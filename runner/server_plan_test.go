package runner

import (
	"testing"
)

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
