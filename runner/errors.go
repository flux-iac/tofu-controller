package runner

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// errShuttingDown is the cancel cause of commands stopped because the runner
// is shutting down.
var errShuttingDown = errors.New("runner shutting down")

// SessionError reports that a request names a Terraform instance the runner
// has no session for. Current is empty when no session exists yet.
type SessionError struct {
	Requested string
	Current   string
}

func (e *SessionError) Error() string {
	if e.Current == "" {
		return fmt.Sprintf("terraform session error: instance id is empty, expected '%s'", e.Requested)
	}

	return fmt.Sprintf("terraform session mismatch: requested instance id '%s' does not match the current instance '%s'", e.Requested, e.Current)
}

// InvalidInputError wraps an error caused by input the caller supplied.
type InvalidInputError struct {
	Err error
}

func (e *InvalidInputError) Error() string { return e.Err.Error() }

func (e *InvalidInputError) Unwrap() error { return e.Err }

// NotLockedError reports that a force-unlock named a lock that is not held.
type NotLockedError struct {
	ID  string
	Err error
}

func (e *NotLockedError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("state lock '%s' is not held", e.ID)
	}

	return e.Err.Error()
}

func (e *NotLockedError) Unwrap() error { return e.Err }

// InterruptedError reports that a terraform command was stopped before it
// finished, for example because the call was cancelled or the runner is
// shutting down.
type InterruptedError struct {
	Reason string
	Err    error
}

func (e *InterruptedError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("terraform interrupted (%s)", e.Reason)
	}

	return fmt.Sprintf("terraform interrupted (%s): %s", e.Reason, e.Err)
}

func (e *InterruptedError) Unwrap() error { return e.Err }

// Detail returns the lock information as a gRPC status detail.
func (e *StateLockError) Detail() *StateLock {
	return &StateLock{
		Id:        e.ID,
		Path:      e.Path,
		Operation: e.Operation,
		Who:       e.Who,
		Version:   e.Version,
		Created:   e.Created,
	}
}

// ToStatus converts an error returned by a handler into a gRPC status error.
// An error that already carries a status other than Unknown is returned as is.
// The runner's server applies it to every call. It is exported so that tests
// of the runner's callers can fake the runner with the errors it really sends.
func ToStatus(err error) error {
	if err == nil {
		return nil
	}

	st, ok := status.FromError(err)
	if ok && st.Code() != codes.Unknown {
		return err
	}

	var (
		lockErr        *StateLockError
		interruptedErr *InterruptedError
		sessionErr     *SessionError
		notLockedErr   *NotLockedError
		invalidErr     *InvalidInputError
	)

	switch {
	case errors.As(err, &lockErr) && lockErr.ID != "":
		return withDetail(codes.Aborted, err, lockErr.Detail())
	case errors.As(err, &interruptedErr):
		return status.Error(codes.Canceled, err.Error())
	case errors.As(err, &sessionErr):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.As(err, &notLockedErr):
		return withDetail(codes.FailedPrecondition, err, &StateLock{Id: notLockedErr.ID})
	case errors.As(err, &invalidErr):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, err.Error())
	case apierrors.IsNotFound(err):
		return status.Error(codes.NotFound, err.Error())
	case apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err):
		return status.Error(codes.PermissionDenied, err.Error())
	default:
		// st.Message() is err.Error(), except for a bare Unknown status,
		// whose own message is kept.
		return status.Error(codes.Internal, st.Message())
	}
}

func withDetail(code codes.Code, err error, detail *StateLock) error {
	st, detailErr := status.New(code, err.Error()).WithDetails(detail)
	if detailErr != nil {
		return status.Error(code, err.Error())
	}

	return st.Err()
}
