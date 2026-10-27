package runner

import (
	"context"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
)

// session is the Terraform object the runner currently works on. NewTerraform
// builds it in full and it is never changed afterwards; the next NewTerraform
// replaces it.
type session struct {
	id        string
	tf        *TerraformExecWrapper
	terraform *infrav1.Terraform
}

type sessionContextKey struct{}

func contextWithSession(ctx context.Context, s *session) context.Context {
	return context.WithValue(ctx, sessionContextKey{}, s)
}

// sessionFrom returns the session the session interceptor put into ctx. It
// panics when there is none, because that means the RPC is missing from
// sessionMethods.
func sessionFrom(ctx context.Context) *session {
	s, ok := ctx.Value(sessionContextKey{}).(*session)
	if !ok || s == nil {
		panic("no terraform session in the request context")
	}

	return s
}

// setSession makes s the current session.
func (r *TerraformRunnerServer) setSession(s *session) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.current = s
}

// checkSession returns the current session when its id is id.
func (r *TerraformRunnerServer) checkSession(id string) (*session, error) {
	r.mu.Lock()
	current := r.current
	r.mu.Unlock()

	if current == nil || current.id == "" {
		return nil, &SessionError{Requested: id}
	}

	if current.id != id {
		return nil, &SessionError{Requested: id, Current: current.id}
	}

	return current, nil
}
