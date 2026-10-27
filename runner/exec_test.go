package runner

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/stretchr/testify/assert"
)

const lockInfoText = `Lock Info:
		ID:        24c59e5b-fa0a-0f35-a5af-841d64651804
		Path:      terraform-bucket/ec2-instance.tfstate
		Operation: OperationTypePlan
		Who:       runner@ec2-instance
		Version:   1.13.1
		Created:   2025-11-06 22:52:22.260153039 +0000 UTC
`

func TestTerraformWrapper_ErrorHandling(t *testing.T) {
	tests := []struct {
		name       string
		inputError error
		wantErr    bool
		wantLock   bool
		wantLockID string
	}{
		{
			// Without a lock holder there is no lock to report or unlock, so
			// the failure is returned as a plain error.
			name:       "Terraform State Lock without Metadata",
			inputError: errors.New("Error acquiring the state lock."),
			wantErr:    true,
			wantLock:   false,
		},
		{
			name:       "Terraform State Lock with Metadata",
			inputError: errors.New(lockInfoText),
			wantErr:    true,
			wantLock:   true,
			wantLockID: "24c59e5b-fa0a-0f35-a5af-841d64651804",
		},
		{
			name:       "Generic Error",
			inputError: errors.New("a generic error"),
			wantErr:    true,
			wantLock:   false,
		},
		{
			name:       "Nil Error",
			inputError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapper := NewTerraformExecWrapper(&tfexec.Terraform{}, 0)

			err := wrapper.normalize(t.Context(), tt.inputError)

			if !tt.wantErr {
				assert.NoError(t, err)
				return
			}

			if !assert.Error(t, err) {
				return
			}

			var sl *StateLockError
			isLock := errors.As(err, &sl)
			assert.Equal(t, tt.wantLock, isLock)
			if tt.wantLock && isLock {
				assert.Equal(t, tt.wantLockID, sl.ID)
			}
		})
	}
}

func TestTerraformWrapper_Metadata(t *testing.T) {
	wrapper := NewTerraformExecWrapper(&tfexec.Terraform{}, 0)

	err := wrapper.normalize(t.Context(), errors.New(lockInfoText))

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

// failedToUnlock is the error force-unlock returns when the backend refuses
// to unlock the state, with msg as the backend's reason.
func failedToUnlock(msg string) error {
	return errors.New("exit status 1\n\nError: Failed to unlock state: " + msg + "\n")
}

func TestNormalize(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	shuttingDown, cancelShutdown := context.WithCancelCause(context.Background())
	cancelShutdown(errShuttingDown)
	pastDeadline, cancelDeadline := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelDeadline()

	emptyStderr := errors.New("exit status 1\n")

	tests := []struct {
		name      string
		normalize func(w *TerraformExecWrapper) error
		check     func(t *testing.T, err error)
	}{
		{
			name: "nil stays nil",
			normalize: func(w *TerraformExecWrapper) error {
				return w.normalize(context.Background(), nil)
			},
			check: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "lock with info is a state lock error",
			normalize: func(w *TerraformExecWrapper) error {
				return w.normalize(context.Background(), errors.New("exit status 1\n\nError: Error acquiring the state lock\n\n"+lockInfoText))
			},
			check: func(t *testing.T, err error) {
				var sl *StateLockError
				if assert.ErrorAs(t, err, &sl) {
					assert.Equal(t, "24c59e5b-fa0a-0f35-a5af-841d64651804", sl.ID)
				}
				assert.ErrorContains(t, errors.Unwrap(err), "Error acquiring the state lock", "the lock error unwraps to terraform's error")
			},
		},
		{
			name: "lock without info is a plain error",
			normalize: func(w *TerraformExecWrapper) error {
				return w.normalize(context.Background(), errors.New("exit status 1\n\nError: Error acquiring the state lock\n\nError message: open terraform.tfstate: permission denied\n"))
			},
			check: func(t *testing.T, err error) {
				var sl *StateLockError
				assert.False(t, errors.As(err, &sl), "not a state lock error")
				assert.ErrorContains(t, err, "permission denied")
			},
		},
		{
			name: "cancelled request is interrupted",
			normalize: func(w *TerraformExecWrapper) error {
				return w.normalize(cancelled, errors.New("exit status 1"))
			},
			check: func(t *testing.T, err error) {
				var ie *InterruptedError
				if assert.ErrorAs(t, err, &ie) {
					assert.Equal(t, "request cancelled", ie.Reason)
				}
				assert.ErrorContains(t, err, "terraform interrupted")
			},
		},
		{
			name: "shutdown is interrupted with its own reason",
			normalize: func(w *TerraformExecWrapper) error {
				return w.normalize(shuttingDown, errors.New("exit status 1"))
			},
			check: func(t *testing.T, err error) {
				var ie *InterruptedError
				if assert.ErrorAs(t, err, &ie) {
					assert.Equal(t, "runner shutting down", ie.Reason)
				}
			},
		},
		{
			name: "past deadline is interrupted with its own reason",
			normalize: func(w *TerraformExecWrapper) error {
				return w.normalize(pastDeadline, errors.New("exit status 1"))
			},
			check: func(t *testing.T, err error) {
				var ie *InterruptedError
				if assert.ErrorAs(t, err, &ie) {
					assert.Equal(t, "deadline exceeded", ie.Reason)
				}
			},
		},
		{
			name: "interrupted force-unlock is not a not-locked error",
			normalize: func(w *TerraformExecWrapper) error {
				return w.normalizeForceUnlock(cancelled, "abc", errors.New("exit status 1\n\nError: Failed to unlock state: LocalState not locked\n"))
			},
			check: func(t *testing.T, err error) {
				var ie *InterruptedError
				assert.ErrorAs(t, err, &ie)
				_, isInterrupted := err.(*InterruptedError)
				assert.True(t, isInterrupted, "the interrupted error is returned as is, got %T", err)
				var nl *NotLockedError
				assert.False(t, errors.As(err, &nl), "not a not-locked error")
			},
		},
		{
			name: "force-unlock of state that is not locked",
			normalize: func(w *TerraformExecWrapper) error {
				return w.normalizeForceUnlock(context.Background(), "abc", errors.New("exit status 1\n\nError: Failed to unlock state: LocalState not locked\n"))
			},
			check: func(t *testing.T, err error) {
				var nl *NotLockedError
				if assert.ErrorAs(t, err, &nl) {
					assert.Equal(t, "abc", nl.ID)
				}
			},
		},
		{
			name: "force-unlock with a different lock id",
			normalize: func(w *TerraformExecWrapper) error {
				return w.normalizeForceUnlock(context.Background(), "abc", errors.New("exit status 1\n\nError: lock ID \"abc\" does not match existing lock ID \"def\"\n"))
			},
			check: func(t *testing.T, err error) {
				var nl *NotLockedError
				assert.ErrorAs(t, err, &nl)
			},
		},
		{
			name: "force-unlock of state the kubernetes backend reports as not locked",
			normalize: func(w *TerraformExecWrapper) error {
				return w.normalizeForceUnlock(context.Background(), "abc", failedToUnlock(`state is already unlocked`))
			},
			check: func(t *testing.T, err error) {
				var nl *NotLockedError
				if assert.ErrorAs(t, err, &nl) {
					assert.Equal(t, "abc", nl.ID)
				}
			},
		},
		{
			name: "force-unlock of state the s3 backend reports as not locked",
			normalize: func(w *TerraformExecWrapper) error {
				return w.normalizeForceUnlock(context.Background(), "abc", failedToUnlock(`failed to retrieve lock info for lock ID "abc": no lock info found for: "bucket/key" within the DynamoDB table: locks`))
			},
			check: func(t *testing.T, err error) {
				var nl *NotLockedError
				if assert.ErrorAs(t, err, &nl) {
					assert.Equal(t, "abc", nl.ID)
				}
			},
		},
		{
			name: "force-unlock of state the azurerm backend reports as not locked",
			normalize: func(w *TerraformExecWrapper) error {
				return w.normalizeForceUnlock(context.Background(), "abc", failedToUnlock(`failed to retrieve lock info: blob metadata "terraformlockid" was empty`))
			},
			check: func(t *testing.T, err error) {
				var nl *NotLockedError
				if assert.ErrorAs(t, err, &nl) {
					assert.Equal(t, "abc", nl.ID)
				}
			},
		},
		{
			name: "force-unlock of state the tfc backend reports as not locked",
			normalize: func(w *TerraformExecWrapper) error {
				return w.normalizeForceUnlock(context.Background(), "abc", failedToUnlock(`workspace already unlocked`))
			},
			check: func(t *testing.T, err error) {
				var nl *NotLockedError
				if assert.ErrorAs(t, err, &nl) {
					assert.Equal(t, "abc", nl.ID)
				}
			},
		},
		{
			name: "force-unlock of state the http backend reports as not locked",
			normalize: func(w *TerraformExecWrapper) error {
				return w.normalizeForceUnlock(context.Background(), "abc", failedToUnlock(`state not locked`))
			},
			check: func(t *testing.T, err error) {
				var nl *NotLockedError
				if assert.ErrorAs(t, err, &nl) {
					assert.Equal(t, "abc", nl.ID)
				}
			},
		},
		{
			name: "force-unlock that fails for another reason is a plain error",
			normalize: func(w *TerraformExecWrapper) error {
				return w.normalizeForceUnlock(context.Background(), "abc", failedToUnlock(`storage: bucket doesn't exist`))
			},
			check: func(t *testing.T, err error) {
				var nl *NotLockedError
				assert.False(t, errors.As(err, &nl), "not a not-locked error")
				assert.ErrorContains(t, err, "bucket doesn't exist")
			},
		},
		{
			name: "not locked outside force-unlock is a plain error",
			normalize: func(w *TerraformExecWrapper) error {
				return w.normalize(context.Background(), errors.New("exit status 1\n\nError: Failed to unlock state: LocalState not locked\n"))
			},
			check: func(t *testing.T, err error) {
				var nl *NotLockedError
				assert.False(t, errors.As(err, &nl))
			},
		},
		{
			name: "generic error with empty stderr keeps the original error",
			normalize: func(w *TerraformExecWrapper) error {
				return w.normalize(context.Background(), emptyStderr)
			},
			check: func(t *testing.T, err error) {
				assert.NotEmpty(t, err.Error())
				assert.ErrorIs(t, err, emptyStderr)
			},
		},
		{
			name: "generic error is sanitised",
			normalize: func(w *TerraformExecWrapper) error {
				return w.normalize(context.Background(), errors.New("exit status 1\n\nError: Invalid value for variable\n    │ var.s is \"SUPERSECRET\"\n"))
			},
			check: func(t *testing.T, err error) {
				assert.NotContains(t, err.Error(), "SUPERSECRET")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, tt.normalize(NewTerraformExecWrapper(&tfexec.Terraform{}, 0)))
		})
	}
}

func TestSanitize(t *testing.T) {
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
		{
			name:     "variable value in a validation error",
			input:    `│ var.s is "SUPERSECRET"`,
			expected: `│ var.s is ***`,
		},
		{
			name: "variable value inside a diagnostic",
			input: `  on main.tf line 1:
   1: variable "s" {
    ├────────────────
    │ var.my-secret_2 is "SUPERSECRET"

too long`,
			expected: `  on main.tf line 1:
   1: variable "s" {
    ├────────────────
    │ var.my-secret_2 is ***

too long`,
		},
		{
			name:     "variable value without the box character",
			input:    `var.s is "SUPERSECRET"`,
			expected: `var.s is ***`,
		}, {
			name:     "variable value of an object attribute",
			input:    `│ var.creds.password is "SUPERSECRET"`,
			expected: `│ var.creds.password is ***`,
		},
		{
			name:     "variable value of an object attribute without the box character",
			input:    `var.creds.password is "SUPERSECRET"`,
			expected: `var.creds.password is ***`,
		},
		{
			name:     "variable value of a list element",
			input:    `│ var.l[0] is "SUPERSECRET"`,
			expected: `│ var.l[0] is ***`,
		},
		{
			name:     "variable value of a list element without the box character",
			input:    `var.l[0] is "SUPERSECRET"`,
			expected: `var.l[0] is ***`,
		},
		{
			name:     "variable value of a map element",
			input:    `│ var.m["k"] is "SUPERSECRET"`,
			expected: `│ var.m["k"] is ***`,
		},
		{
			name:     "variable value of a map element without the box character",
			input:    `var.m["k"] is "SUPERSECRET"`,
			expected: `var.m["k"] is ***`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, sanitize(tt.input))
		})
	}
}

func TestWaitDelay(t *testing.T) {
	tests := []struct {
		grace time.Duration
		want  time.Duration
	}{
		{grace: 0, want: 60 * time.Second},
		{grace: 30 * time.Second, want: 60 * time.Second},
		{grace: 300 * time.Second, want: 300 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.grace.String(), func(t *testing.T) {
			wrapper := NewTerraformExecWrapper(&tfexec.Terraform{}, tt.grace)
			assert.Equal(t, tt.want, WaitDelay(wrapper))
		})
	}
}

func TestSanitizingWriter(t *testing.T) {
	const input = "Error: bad\n  on generated.auto.tfvars.json line 1:\n   1: {\"s\":\"SUPERSECRET\"}\n" +
		"│ var.creds.password is \"SUPERSECRET\"\n│ var.l[0] is \"SUPERSECRET\"\n│ var.m[\"k\"] is \"SUPERSECRET\"\n" +
		"var.creds.password is \"SUPERSECRET\"\nvar.l[0] is \"SUPERSECRET\"\nvar.m[\"k\"] is \"SUPERSECRET\"\n" +
		"│ var.s is \"SUPERSECRET\""
	const want = "Error: bad\n  on generated.auto.tfvars.json line 1:\n   1: {\"s\":\"***\"}\n" +
		"│ var.creds.password is ***\n│ var.l[0] is ***\n│ var.m[\"k\"] is ***\n" +
		"var.creds.password is ***\nvar.l[0] is ***\nvar.m[\"k\"] is ***\n" +
		"│ var.s is ***"

	var out bytes.Buffer
	w := &sanitizingWriter{w: &out}
	// Byte by byte, so lines arrive split across writes.
	for i := range len(input) {
		n, err := w.Write([]byte{input[i]})
		assert.NoError(t, err)
		assert.Equal(t, 1, n)
	}
	assert.NotContains(t, out.String(), "var.s", "an unfinished line is held back")
	w.flush()

	assert.Equal(t, want, out.String())
}
