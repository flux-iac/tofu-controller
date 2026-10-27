package runner_test

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"

	. "github.com/onsi/gomega"
)

// harness serves a TerraformRunnerServer in-process over bufconn, backed by a
// fake Kubernetes client and a per-test directory.
type harness struct {
	Client runner.RunnerClient
	Server *runner.TerraformRunnerServer
	Kube   client.WithWatch
	Dir    string
	// Logs holds everything the server logged while handling requests.
	Logs *syncBuffer
	// ServeDone closes when runner.Serve returns.
	ServeDone <-chan struct{}

	stop     chan struct{}
	stopOnce sync.Once
	serveErr error
}

// ServeErr returns what runner.Serve returned. Call it only once ServeDone
// has closed.
func (h *harness) ServeErr() error {
	return h.serveErr
}

// Stop tells the server to shut down, as SIGTERM does in production.
func (h *harness) Stop() {
	h.stopOnce.Do(func() { close(h.stop) })
}

// syncBuffer is a bytes.Buffer that is safe for concurrent writes and reads,
// because server goroutines log while the test reads.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) WriteString(s string) (int, error) {
	return b.Write([]byte(s))
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// harnessOption customises how newHarnessWith builds the harness.
type harnessOption func(*harnessConfig)

type harnessConfig struct {
	interceptors *interceptor.Funcs
	shutdown     <-chan struct{}
}

// withShutdown gives the server shutdown as its Shutdown channel in place of
// the one runner.Serve closes, so a test can shut the server down while it
// still takes calls.
func withShutdown(shutdown <-chan struct{}) harnessOption {
	return func(c *harnessConfig) { c.shutdown = shutdown }
}

// withInterceptors makes the fake Kubernetes client call funcs, so a test can
// inject failures.
func withInterceptors(funcs interceptor.Funcs) harnessOption {
	return func(c *harnessConfig) { c.interceptors = &funcs }
}

func newHarness(t *testing.T, objs ...client.Object) *harness {
	t.Helper()

	return newHarnessWith(t, nil, objs...)
}

func newHarnessWith(t *testing.T, options []harnessOption, objs ...client.Object) *harness {
	t.Helper()

	var cfg harnessConfig
	for _, o := range options {
		o(&cfg)
	}

	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		clientgoscheme.AddToScheme,
		sourcev1.AddToScheme,
		infrav1.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}

	builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...)
	if cfg.interceptors != nil {
		builder = builder.WithInterceptorFuncs(*cfg.interceptors)
	}
	kube := builder.Build()

	dir := t.TempDir()
	shutdown := make(chan struct{})
	server := &runner.TerraformRunnerServer{
		Shutdown:       shutdown,
		Client:         kube,
		Scheme:         scheme,
		HomeDir:        filepath.Join(dir, "home"),
		TempDir:        filepath.Join(dir, "tmp"),
		BreakGlassFile: filepath.Join(dir, ".break-glass"),
	}
	if cfg.shutdown != nil {
		server.Shutdown = cfg.shutdown
	}
	for _, d := range []string{server.HomeDir, server.TempDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	logs := &syncBuffer{}
	logger := funcr.New(func(prefix, args string) {
		logs.WriteString(prefix + " " + args + "\n")
	}, funcr.Options{})

	// The log-injecting interceptor runs first, so the production
	// interceptors after it log to the captured logger.
	chain := append([]grpc.UnaryServerInterceptor{loggerInterceptor(logger)}, server.UnaryInterceptors()...)
	grpcServer := grpc.NewServer(grpc.ChainUnaryInterceptor(chain...))
	runner.RegisterRunnerServer(grpcServer, server)

	lis := bufconn.Listen(1 << 20)
	stop := make(chan struct{})
	serveDone := make(chan struct{})

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}

	h := &harness{
		Client:    runner.NewRunnerClient(conn),
		Server:    server,
		Kube:      kube,
		Dir:       dir,
		Logs:      logs,
		ServeDone: serveDone,
		stop:      stop,
	}
	go func() {
		defer close(serveDone)
		h.serveErr = runner.Serve(lis, grpcServer, shutdown, stop)
	}()

	t.Cleanup(func() {
		_ = conn.Close()
		h.Stop()
		grpcServer.Stop()
		<-serveDone
		_ = lis.Close()
	})

	return h
}

// loggerInterceptor puts logger into every request context, as the
// controller-runtime logger the handlers read with ctrl.LoggerFrom.
func loggerInterceptor(logger logr.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return handler(ctrl.LoggerInto(ctx, logger), req)
	}
}

// newSession creates a Terraform instance on the server and returns its id.
func (h *harness) newSession(t *testing.T, tf *infrav1.Terraform, execPath, workingDir string) string {
	t.Helper()

	b, err := tf.ToBytes(h.Server.Scheme)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	if _, err := h.Client.NewTerraform(t.Context(), &runner.NewTerraformRequest{
		WorkingDir: workingDir,
		ExecPath:   execPath,
		Terraform:  b,
		InstanceID: id,
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestHarness_PathsAreInjected(t *testing.T) {
	g := NewWithT(t)
	h := newHarness(t)

	_, err := h.Client.CreateFileMappings(t.Context(), &runner.CreateFileMappingsRequest{
		FileMappings: []*runner.FileMapping{{
			Location: "home",
			Path:     "a/b",
			Content:  []byte("hello"),
		}},
	})
	g.Expect(err).NotTo(HaveOccurred())

	info, err := os.Stat(filepath.Join(h.Dir, "home", "a/b"))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(info.Mode().Perm()).To(Equal(os.FileMode(0600)))

	_, err = h.Client.StartBreakTheGlassSession(t.Context(), &runner.BreakTheGlassRequest{})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(h.Server.BreakGlassFile).To(Equal(filepath.Join(h.Dir, ".break-glass")))
	g.Expect(h.Server.BreakGlassFile).To(BeAnExistingFile())
}

func TestHarness_CapturesServerLogs(t *testing.T) {
	g := NewWithT(t)
	h := newHarness(t)

	_, err := h.Client.StartBreakTheGlassSession(t.Context(), &runner.BreakTheGlassRequest{})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(h.Logs.String()).To(ContainSubstring("starting break the glass session"))
}
