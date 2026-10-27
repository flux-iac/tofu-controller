package runner

import (
	"context"
	"fmt"
	"path"
	"runtime/debug"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	ctrl "sigs.k8s.io/controller-runtime"
)

// sessionMethods are the RPCs that work on the current session. The session
// interceptor checks their TfInstance against it before the handler runs.
var sessionMethods = map[string]bool{
	Runner_SetEnv_FullMethodName:            true,
	Runner_Init_FullMethodName:              true,
	Runner_SelectWorkspace_FullMethodName:   true,
	Runner_Plan_FullMethodName:              true,
	Runner_ShowPlanFile_FullMethodName:      true,
	Runner_ShowPlanFileRaw_FullMethodName:   true,
	Runner_SaveTFPlan_FullMethodName:        true,
	Runner_LoadTFPlan_FullMethodName:        true,
	Runner_Apply_FullMethodName:             true,
	Runner_Destroy_FullMethodName:           true,
	Runner_Output_FullMethodName:            true,
	Runner_GetInventory_FullMethodName:      true,
	Runner_GenerateVarsForTF_FullMethodName: true,
	Runner_GenerateTemplate_FullMethodName:  true,
	Runner_ForceUnlock_FullMethodName:       true,
}

// tfInstanceRequest is a request that names the Terraform instance it is for.
type tfInstanceRequest interface {
	GetTfInstance() string
}

// UnaryInterceptors returns the interceptors every runner gRPC server
// installs, outermost first: panic recovery, the mapping of errors to gRPC
// status codes, logging, cancellation on shutdown and the session check.
// Status mapping wraps both the session check and the handler, so every error
// is mapped in one place, and logging records the error before it is mapped.
func (r *TerraformRunnerServer) UnaryInterceptors() []grpc.UnaryServerInterceptor {
	return []grpc.UnaryServerInterceptor{
		recoveryInterceptor,
		statusInterceptor,
		loggingInterceptor,
		r.shutdownInterceptor,
		r.sessionInterceptor,
	}
}

// recoveryInterceptor turns a panic in a handler into an Internal error, so
// the call fails and the runner keeps serving.
func recoveryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	defer func() {
		if p := recover(); p != nil {
			ctrl.LoggerFrom(ctx).WithName("runner").Error(fmt.Errorf("%v", p), "rpc panicked",
				"rpc", path.Base(info.FullMethod), "stack", string(debug.Stack()))
			resp = nil
			err = status.Errorf(codes.Internal, "runner panic in %s: %v", info.FullMethod, p)
		}
	}()

	return handler(ctx, req)
}

// loggingInterceptor gives each call a logger that names the RPC and the
// Terraform instance, and logs the error a call fails with.
func loggingInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	log := ctrl.LoggerFrom(ctx).WithName("runner").WithValues("rpc", path.Base(info.FullMethod))
	if withID, ok := req.(tfInstanceRequest); ok {
		log = log.WithValues("instance-id", withID.GetTfInstance())
	}
	ctx = ctrl.LoggerInto(ctx, log)

	log.V(1).Info("rpc started")
	resp, err := handler(ctx, req)
	if err != nil {
		log.Error(err, "rpc failed")
	}

	return resp, err
}

// sessionInterceptor checks that a session RPC names the current session and
// puts that session into the context for the handler.
func (r *TerraformRunnerServer) sessionInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if !sessionMethods[info.FullMethod] {
		return handler(ctx, req)
	}

	var id string
	if withID, ok := req.(tfInstanceRequest); ok {
		id = withID.GetTfInstance()
	}

	s, err := r.checkSession(id)
	if err != nil {
		return nil, err
	}

	return handler(contextWithSession(ctx, s), req)
}

// shutdownInterceptor cancels the call's context with errShuttingDown when
// Shutdown closes, so the terraform command the call runs is interrupted and
// reports that the runner is shutting down.
func (r *TerraformRunnerServer) shutdownInterceptor(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	// A call that arrives once shutdown has begun is cancelled before its
	// handler runs, so it never starts terraform.
	select {
	case <-r.Shutdown:
		cancel(errShuttingDown)
	default:
	}

	go func() {
		select {
		case <-r.Shutdown:
			cancel(errShuttingDown)
		case <-ctx.Done():
		}
	}()

	return handler(ctx, req)
}

// statusInterceptor maps the error a call fails with to a gRPC status.
func statusInterceptor(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	resp, err := handler(ctx, req)

	return resp, ToStatus(err)
}
