package controllers

import (
	"errors"
	"fmt"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// QuotaExhaustedError wraps a Kubernetes quota error with retry metadata.
type QuotaExhaustedError struct {
	WrappedErr error
	RetryAfter time.Duration
}

// Error implements the error interface.
func (e *QuotaExhaustedError) Error() string {
	return fmt.Sprintf("quota exhausted: %v", e.WrappedErr)
}

// Unwrap returns the wrapped error for error chain traversal.
func (e *QuotaExhaustedError) Unwrap() error {
	return e.WrappedErr
}

// NewQuotaExhaustedError creates a new QuotaExhaustedError with the given retry delay.
func NewQuotaExhaustedError(wrappedErr error, retryDelay time.Duration) *QuotaExhaustedError {
	return &QuotaExhaustedError{
		WrappedErr: wrappedErr,
		RetryAfter: retryDelay,
	}
}

// IsQuotaExhausted checks if an error is a QuotaExhaustedError using errors.As.
func IsQuotaExhausted(err error) (*QuotaExhaustedError, bool) {
	var qe *QuotaExhaustedError
	return qe, errors.As(err, &qe)
}

// DetectQuotaError checks if a Kubernetes API error is a quota exhaustion error.
// It identifies quota errors by checking for Forbidden status and quota-related keywords.
func DetectQuotaError(err error) bool {
	if !apierrors.IsForbidden(err) {
		return false
	}

	errMsg := err.Error()
	quotaPatterns := []string{
		"quota",
		"exceeded",
		"resource limit",
		"pod limit",
	}

	// Check for quota patterns in the error message
	for _, pattern := range quotaPatterns {
		if strings.Contains(strings.ToLower(errMsg), pattern) {
			return true
		}
	}

	// Check Kubernetes StatusError reason for quota indicators
	if statusErr, ok := err.(*apierrors.StatusError); ok {
		if statusErr.Status().Reason == metav1.StatusReasonForbidden {
			return strings.Contains(statusErr.Status().Message, "quota")
		}
	}

	return false
}
