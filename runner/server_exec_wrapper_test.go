package runner

import (
	"errors"
	reflect "reflect"
	"testing"

	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/stretchr/testify/assert"
)

func TestTerraformWrapper_ErrorHandling(t *testing.T) {
	tests := []struct {
		name          string
		inputError    error
		wantErr       bool
		expectedError error
	}{
		{
			name:          "Terraform State Lock without Metadata",
			inputError:    errors.New("Error acquiring the state lock."),
			wantErr:       true,
			expectedError: &StateLockError{},
		},
		{
			name: "Terraform State Lock with Metadata",
			inputError: errors.New(`Lock Info:
		ID:        24c59e5b-fa0a-0f35-a5af-841d64651804
		Path:      terraform-bucket/ec2-instance.tfstate
		Operation: OperationTypePlan
		Who:       runner@ec2-instance
		Version:   1.13.1
		Created:   2025-11-06 22:52:22.260153039 +0000 UTC
`),
			wantErr: true,
			expectedError: &StateLockError{
				ID:        "24c59e5b-fa0a-0f35-a5af-841d64651804",
				Path:      "terraform-bucket/ec2-instance.tfstate",
				Operation: "OperationTypePlan",
				Who:       "runner@ec2-instance",
				Version:   "1.13.1",
				Created:   "2025-11-06 22:52:22.260153039 +0000 UTC",
			},
		},
		{
			name:          "Generic Error",
			inputError:    errors.New("a generic error"),
			wantErr:       true,
			expectedError: errors.New("a generic error"),
		},
		{
			name:       "Nil Error",
			inputError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapper := NewTerraformExecWrapper(&tfexec.Terraform{})

			err := wrapper.NormalizeError(tt.inputError)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error of type %T, got nil", tt.expectedError)
				}

				if tt.expectedError != nil {
					target := reflect.New(reflect.TypeOf(tt.expectedError).Elem()).Interface()
					if !errors.As(err, &target) {
						t.Fatalf("expected error type %T, got %T", tt.expectedError, err)
					}
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got %v (%T)", err, err)
				}
			}
		})
	}
}

func TestTerraformWrapper_Metadata(t *testing.T) {
	wrapper := NewTerraformExecWrapper(&tfexec.Terraform{})

	err := wrapper.NormalizeError(errors.New(`Lock Info:
		ID:        24c59e5b-fa0a-0f35-a5af-841d64651804
		Path:      terraform-bucket/ec2-instance.tfstate
		Operation: OperationTypePlan
		Who:       runner@ec2-instance
		Version:   1.13.1
		Created:   2025-11-06 22:52:22.260153039 +0000 UTC
`))

	var stateLockErr *StateLockError
	if !errors.As(err, &stateLockErr) {
		t.Fatalf("expected StateLockError, got %T", err)
	}

	assert.Equal(t, "24c59e5b-fa0a-0f35-a5af-841d64651804", stateLockErr.ID)
	assert.Equal(t, "terraform-bucket/ec2-instance.tfstate", stateLockErr.Path)
	assert.Equal(t, "OperationTypePlan", stateLockErr.Operation)
	assert.Equal(t, "runner@ec2-instance", stateLockErr.Who)
	assert.Equal(t, "1.13.1", stateLockErr.Version)
	assert.Equal(t, "2025-11-06 22:52:22.260153039 +0000 UTC", stateLockErr.Created)
}
