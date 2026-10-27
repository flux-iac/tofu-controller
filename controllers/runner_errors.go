package controllers

import (
	"context"

	"github.com/flux-iac/tofu-controller/runner"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// runnerError is an error returned by a runner call. Its text is the status
// message alone, without the "rpc error: code = ... desc = " prefix, because
// that text reaches conditions and events. It keeps the status, so
// status.Code and status.FromError still see the code and details.
type runnerError struct {
	st *status.Status
}

func (e *runnerError) Error() string { return e.st.Message() }

func (e *runnerError) GRPCStatus() *status.Status { return e.st }

// newRunnerError converts the error of a runner call into a *runnerError. An
// error that carries no status becomes an Unknown status with its own text.
func newRunnerError(err error) error {
	if err == nil {
		return nil
	}

	return &runnerError{st: status.Convert(err)}
}

// runnerErrorInterceptor converts every error a runner call returns with
// newRunnerError.
func runnerErrorInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		return newRunnerError(invoker(ctx, method, req, reply, cc, opts...))
	}
}

// stateLockFrom returns the lock that made a runner call fail: the call
// failed with Aborted and carries a StateLock detail.
func stateLockFrom(err error) (*runner.StateLock, bool) {
	return stateLockDetail(err, codes.Aborted)
}

// isNotLocked reports whether a ForceUnlock failed because the lock is not
// held. The runner reports that as FailedPrecondition with a StateLock
// detail; a missing or mismatched session is also FailedPrecondition, but
// carries no detail.
func isNotLocked(err error) bool {
	_, ok := stateLockDetail(err, codes.FailedPrecondition)
	return ok
}

func stateLockDetail(err error, code codes.Code) (*runner.StateLock, bool) {
	st, ok := status.FromError(err)
	if !ok || st.Code() != code {
		return nil, false
	}

	for _, detail := range st.Details() {
		if lock, ok := detail.(*runner.StateLock); ok {
			return lock, true
		}
	}

	return nil, false
}
