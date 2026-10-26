package controllers

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/status"
)

// errRunnerUnreachable is the cancellation cause recorded when a runner call is
// abandoned because the connection to the runner stopped being READY.
var errRunnerUnreachable = errors.New("runner unreachable")

// runnerConnectionState is shared by every call on one runner connection.
type runnerConnectionState struct {
	// reached is set once the connection has been READY or a call has
	// succeeded, so the runner is known to have been up.
	reached atomic.Bool
	// unreachable is set when a call was abandoned because the runner went
	// away, and cleared once the connection is READY again.
	unreachable atomic.Bool
}

// runnerUnreachableInterceptor cancels a runner call once the connection to a
// runner it has already reached has not been READY for longer than timeout. A
// timeout of zero or less disables it. Build one per connection: the state it
// keeps describes a single runner.
//
// The runner client is created with waitForReady:true, so a call on a connection
// whose runner has gone away blocks until its context is done instead of failing
// with UNAVAILABLE. That includes the retry gRPC makes when an in-flight call
// fails because the runner process died mid-plan. Plan, Apply and the other
// calls made after setupTerraform run on the reconcile context, which has no
// deadline, so such a call parked the reconcile forever and only a controller
// restart freed it.
//
// A fixed deadline would also cut legitimately long plans and applies. Those
// keep the connection READY for their whole duration, so watching the
// connection fires only when the runner is actually unreachable.
func runnerUnreachableInterceptor(timeout time.Duration) grpc.UnaryClientInterceptor {
	if timeout <= 0 {
		return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
	}

	conn := &runnerConnectionState{}
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		// An earlier call already gave up on this runner. Fail the calls that
		// follow it, such as the deferred CleanupDir, instead of waiting out the
		// timeout again.
		if state := cc.GetState(); conn.unreachable.Load() && state != connectivity.Ready {
			if state == connectivity.Idle {
				// Keep trying, so that a recovered connection shows as READY again.
				cc.Connect()
			}
			return status.Errorf(codes.Unavailable, "%s: %v", method, errRunnerUnreachable)
		}

		callCtx, cancel := context.WithCancelCause(ctx)
		defer cancel(nil)
		go watchRunnerConnection(callCtx, cc, timeout, conn, cancel)

		err := invoker(callCtx, method, req, reply, cc, opts...)
		if err == nil {
			conn.reached.Store(true)
			return nil
		}
		if cause := context.Cause(callCtx); errors.Is(cause, errRunnerUnreachable) {
			conn.unreachable.Store(true)
			return status.Errorf(codes.Unavailable, "%s: %v", method, cause)
		}
		return err
	}
}

// watchRunnerConnection calls cancel with errRunnerUnreachable once cc, having
// reached the runner, has been out of the READY state for timeout without
// interruption. It returns when ctx is done, which happens when the call
// finishes.
func watchRunnerConnection(ctx context.Context, cc *grpc.ClientConn, timeout time.Duration, conn *runnerConnectionState, cancel context.CancelCauseFunc) {
	var notReadySince time.Time
	for {
		state := cc.GetState()
		switch {
		case state == connectivity.Ready:
			conn.reached.Store(true)
			conn.unreachable.Store(false)
			notReadySince = time.Time{}
			cc.WaitForStateChange(ctx, state)

		case !conn.reached.Load():
			// The runner is still starting. Its pod gets an IP before the image is
			// pulled, so a first call can legitimately wait a long time. That wait
			// is left to the caller's deadline; setupTerraform bounds its calls
			// with RunnerRPCTimeout.
			cc.WaitForStateChange(ctx, state)

		default:
			if state == connectivity.Idle {
				// A GOAWAY leaves the channel IDLE while the call drains on the old
				// transport. Reconnect so that a live runner shows as READY again.
				// The runner never sends GOAWAY itself; if a proxy in front of it
				// does and reconnecting keeps failing for timeout, the call is
				// cancelled even though the old transport is still draining.
				cc.Connect()
			}
			if notReadySince.IsZero() {
				notReadySince = time.Now()
			}
			remaining := timeout - time.Since(notReadySince)
			if remaining <= 0 {
				cancel(fmt.Errorf("%w: connection not ready for %s (state %s)", errRunnerUnreachable, timeout, state))
				return
			}
			waitCtx, waitCancel := context.WithTimeout(ctx, remaining)
			cc.WaitForStateChange(waitCtx, state)
			waitCancel()
		}

		if ctx.Err() != nil {
			return
		}
	}
}
