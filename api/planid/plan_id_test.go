package planid

import (
	"testing"
)

func TestGetPlanID(t *testing.T) {
	tests := []struct {
		name     string
		revision string
		want     string
	}{
		{
			name:     "Valid hash more than 10 characters",
			revision: "branch1@algo1:12345678901234567890",
			want:     "plan-branch1-1234567890",
		},
		{
			name:     "Valid hash equal to 10 characters",
			revision: "branch2@algo2:1234567890",
			want:     "plan-branch2-1234567890",
		},
		{
			name:     "Valid hash less than 10 characters",
			revision: "branch3@algo3:123456789",
			want:     "plan-branch3-123456789",
		},
		{
			name:     "Valid branch and hash",
			revision: "branch4@algo4:abc123xyz",
			want:     "plan-branch4-abc123xyz",
		},
		{
			name:     "Valid old format",
			revision: "main/12345678901234567890",
			want:     "plan-main-1234567890",
		},
		{
			name:     "Valid old format bucket format",
			revision: "12345678901234567890",
			want:     "plan-1234567890",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetPlanID(tt.revision); got != tt.want {
				t.Errorf("GetPlanID() = %v, want %v", got, tt.want)
			}
		})
	}
}
