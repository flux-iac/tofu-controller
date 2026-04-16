package controllers

import (
	"errors"
	"fmt"
	"math/rand"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestDetectQuotaError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "quota error with 'quota' keyword",
			err:  apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("exceeded quota")),
			want: true,
		},
		{
			name: "quota error with 'exceeded' keyword",
			err:  apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("resource exceeded")),
			want: true,
		},
		{
			name: "non-forbidden error",
			err:  apierrors.NewNotFound(schema.GroupResource{}, "test"),
			want: false,
		},
		{
			name: "forbidden but not quota",
			err:  apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("permission denied")),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectQuotaError(tt.err)
			if got != tt.want {
				t.Errorf("DetectQuotaError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsQuotaExhausted(t *testing.T) {
	baseErr := apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("exceeded quota"))
	quotaErr := NewQuotaExhaustedError(baseErr, 5*time.Second)

	tests := []struct {
		name   string
		err    error
		wantOK bool
	}{
		{
			name:   "quota error",
			err:    quotaErr,
			wantOK: true,
		},
		{
			name:   "wrapped quota error",
			err:    fmt.Errorf("wrapper: %w", quotaErr),
			wantOK: true,
		},
		{
			name:   "non-quota error",
			err:    baseErr,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := IsQuotaExhausted(tt.err)
			if ok != tt.wantOK {
				t.Errorf("IsQuotaExhausted() ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got.RetryAfter != 5*time.Second {
				t.Errorf("IsQuotaExhausted() RetryAfter = %v, want 5s", got.RetryAfter)
			}
		})
	}
}

func TestQuotaExhaustedErrorUnwrap(t *testing.T) {
	baseErr := apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("exceeded quota"))
	quotaErr := NewQuotaExhaustedError(baseErr, 5*time.Second)

	if !errors.Is(quotaErr, baseErr) {
		t.Errorf("Unwrap() did not preserve wrapped error")
	}
}

func TestQuotaRetryJitterCalculation(t *testing.T) {
	baseDelay := 5 * time.Second
	jitterMax := 2 * time.Second

	// Simulate the jitter calculation
	for i := 0; i < 10; i++ {
		jitter := time.Duration(rand.Intn(int(jitterMax.Milliseconds()))) * time.Millisecond
		totalDelay := baseDelay + jitter

		// Total delay should be >= baseDelay and < baseDelay + jitterMax
		if totalDelay < baseDelay || totalDelay >= baseDelay+jitterMax {
			t.Errorf("iteration %d: jitter calculation out of bounds: %v (expected [%v, %v))",
				i, totalDelay, baseDelay, baseDelay+jitterMax)
		}
	}
}

func TestQuotaErrorChaining(t *testing.T) {
	baseErr := apierrors.NewForbidden(schema.GroupResource{}, "test", errors.New("exceeded quota"))
	quotaErr := NewQuotaExhaustedError(baseErr, 5*time.Second)

	// Test that the error is properly chained
	if quotaErr.Error() != fmt.Sprintf("quota exhausted: %v", baseErr) {
		t.Errorf("Error() returned unexpected string: %s", quotaErr.Error())
	}

	// Test that Unwrap preserves the original error
	if quotaErr.Unwrap() != baseErr {
		t.Errorf("Unwrap() did not return the original error")
	}

	// Test that errors.Is works with wrapped errors
	if !errors.Is(quotaErr, baseErr) {
		t.Errorf("errors.Is() failed to find wrapped error")
	}
}
