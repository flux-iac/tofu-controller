package runner

import "time"

// Shims that let the external test package reach unexported functions.
var (
	CtyValueToGoValue               = ctyValueToGoValue
	GetInventoryFromTerraformModule = getInventoryFromTerraformModule
	TfVarsFileOptions               = tfVarsFileOptions
)

// WaitDelay returns the delay the wrapper gave terraform to exit after an
// interrupt.
func WaitDelay(t *TerraformExecWrapper) time.Duration { return t.waitDelay }

// SessionWaitDelay returns the delay the current session's wrapper gives
// terraform to exit after an interrupt, or zero when there is no session.
func SessionWaitDelay(r *TerraformRunnerServer) time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.current == nil {
		return 0
	}

	return r.current.tf.waitDelay
}
