package runner

import (
	"errors"
	"os"
	"sync"

	"net"

	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	TFPlanName                            = "tfplan"
	SavedPlanSecretAnnotation             = "savedPlan"
	runnerFileMappingLocationHome         = "home"
	runnerFileMappingLocationWorkspace    = "workspace"
	runnerFileMappingDirectoryPermissions = 0700
	runnerFileMappingFilePermissions      = 0600
	HomePath                              = "/home/runner"
)

type TerraformRunnerServer struct {
	UnimplementedRunnerServer
	client.Client
	Scheme *runtime.Scheme
	// Shutdown closes when the runner shuts down. Every call in flight is then
	// cancelled, which interrupts the terraform command it runs. A nil
	// Shutdown never closes.
	Shutdown <-chan struct{}

	// mu guards current, the session that NewTerraform last created.
	mu      sync.Mutex
	current *session

	// HomeDir, TempDir and BreakGlassFile override the default filesystem
	// locations. An empty value selects the production default.
	HomeDir        string
	TempDir        string
	BreakGlassFile string
}

func (r *TerraformRunnerServer) homeDir() string {
	if r.HomeDir != "" {
		return r.HomeDir
	}
	return HomePath
}

func (r *TerraformRunnerServer) tempDir() string {
	if r.TempDir != "" {
		return r.TempDir
	}
	return os.TempDir()
}

func (r *TerraformRunnerServer) breakGlassFile() string {
	if r.BreakGlassFile != "" {
		return r.BreakGlassFile
	}
	return BreakGlassFile
}

// Serve serves srv on lis until stop closes or serving fails. It then closes
// shutdown, which must be the Shutdown channel of the server registered on
// srv, so every running command is interrupted, and waits for the calls in
// flight to return. Serve is the only one that closes shutdown, and it closes
// it once. A stop returns nil, even one that arrives before srv has started
// serving.
//
// Waiting for the calls in flight is bounded only because every handler
// honours its cancelled context and terraform runs with a WaitDelay, after
// which it is killed.
func Serve(lis net.Listener, srv *grpc.Server, shutdown chan struct{}, stop <-chan struct{}) error {
	served := make(chan error, 1)
	go func() { served <- srv.Serve(lis) }()

	select {
	case <-stop:
		close(shutdown)
		srv.GracefulStop()
		// When the stop came before srv started serving, Serve finds the
		// server already stopped.
		if err := <-served; !errors.Is(err, grpc.ErrServerStopped) {
			return err
		}
		return nil
	case err := <-served:
		close(shutdown)
		srv.GracefulStop()
		return err
	}
}
