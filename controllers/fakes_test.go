package controllers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/patch"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/hashicorp/go-retryablehttp"
	"google.golang.org/grpc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// Test fakes shared by the unit tests that drive step functions without
// envtest's runner: a recording runner client, a patcher on a fake API server,
// and an artifact server.

// fakeRunner is a runner.RunnerClient that records every RPC it receives, in
// order. Each RPC returns the error in errs[name] if set, else the reply in
// replies[name] if set (it must be the RPC's reply type), else a default reply.
// Errors go through the runner server's status mapping and then the
// conversion runnerErrorInterceptor applies on a real connection, so tests see
// the errors the reconciler sees in production: a plain Go error arrives as
// Internal, as any unexpected handler error does.
// RPCs it does not override hit the nil embedded interface and panic, so an
// unexpected call fails the test loudly.
type fakeRunner struct {
	runner.RunnerClient

	mu       sync.Mutex
	calls    []string
	requests map[string][]any
	errs     map[string]error
	replies  map[string]any

	// onCall, when set, is called with the RPC name before it returns. Tests use
	// it to record RPCs alongside other events in one ordered log.
	onCall func(name string)
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{
		requests: map[string][]any{},
		errs:     map[string]error{},
		replies:  map[string]any{},
	}
}

// Calls returns the RPC names received so far, in order.
func (f *fakeRunner) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// Requests returns the requests received for one RPC, in order.
func (f *fakeRunner) Requests(name string) []any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]any(nil), f.requests[name]...)
}

func fakeRPC[T any](f *fakeRunner, name string, req any, def *T) (*T, error) {
	f.mu.Lock()
	f.calls = append(f.calls, name)
	f.requests[name] = append(f.requests[name], req)
	err := f.errs[name]
	r, hasReply := f.replies[name]
	onCall := f.onCall
	f.mu.Unlock()

	if onCall != nil {
		onCall(name)
	}
	if err != nil {
		return nil, newRunnerError(runner.ToStatus(err))
	}
	if hasReply {
		return r.(*T), nil
	}
	return def, nil
}

func (f *fakeRunner) LookPath(_ context.Context, in *runner.LookPathRequest, _ ...grpc.CallOption) (*runner.LookPathReply, error) {
	return fakeRPC(f, "LookPath", in, &runner.LookPathReply{ExecPath: "/usr/bin/tofu"})
}

func (f *fakeRunner) NewTerraform(_ context.Context, in *runner.NewTerraformRequest, _ ...grpc.CallOption) (*runner.NewTerraformReply, error) {
	return fakeRPC(f, "NewTerraform", in, &runner.NewTerraformReply{Id: in.InstanceID})
}

func (f *fakeRunner) SetEnv(_ context.Context, in *runner.SetEnvRequest, _ ...grpc.CallOption) (*runner.SetEnvReply, error) {
	return fakeRPC(f, "SetEnv", in, &runner.SetEnvReply{})
}

func (f *fakeRunner) CreateFileMappings(_ context.Context, in *runner.CreateFileMappingsRequest, _ ...grpc.CallOption) (*runner.CreateFileMappingsReply, error) {
	return fakeRPC(f, "CreateFileMappings", in, &runner.CreateFileMappingsReply{})
}

func (f *fakeRunner) UploadAndExtract(_ context.Context, in *runner.UploadAndExtractRequest, _ ...grpc.CallOption) (*runner.UploadAndExtractReply, error) {
	return fakeRPC(f, "UploadAndExtract", in, &runner.UploadAndExtractReply{WorkingDir: "/fake/work", TmpDir: "/fake/tmp"})
}

func (f *fakeRunner) CleanupDir(_ context.Context, in *runner.CleanupDirRequest, _ ...grpc.CallOption) (*runner.CleanupDirReply, error) {
	return fakeRPC(f, "CleanupDir", in, &runner.CleanupDirReply{Message: "ok"})
}

func (f *fakeRunner) WriteBackendConfig(_ context.Context, in *runner.WriteBackendConfigRequest, _ ...grpc.CallOption) (*runner.WriteBackendConfigReply, error) {
	return fakeRPC(f, "WriteBackendConfig", in, &runner.WriteBackendConfigReply{Message: "ok"})
}

func (f *fakeRunner) ProcessCliConfig(_ context.Context, in *runner.ProcessCliConfigRequest, _ ...grpc.CallOption) (*runner.ProcessCliConfigReply, error) {
	return fakeRPC(f, "ProcessCliConfig", in, &runner.ProcessCliConfigReply{FilePath: "/fake/work/.terraformrc"})
}

func (f *fakeRunner) GenerateVarsForTF(_ context.Context, in *runner.GenerateVarsForTFRequest, _ ...grpc.CallOption) (*runner.GenerateVarsForTFReply, error) {
	return fakeRPC(f, "GenerateVarsForTF", in, &runner.GenerateVarsForTFReply{Message: "ok"})
}

func (f *fakeRunner) GenerateTemplate(_ context.Context, in *runner.GenerateTemplateRequest, _ ...grpc.CallOption) (*runner.GenerateTemplateReply, error) {
	return fakeRPC(f, "GenerateTemplate", in, &runner.GenerateTemplateReply{Message: "ok"})
}

func (f *fakeRunner) Init(_ context.Context, in *runner.InitRequest, _ ...grpc.CallOption) (*runner.InitReply, error) {
	return fakeRPC(f, "Init", in, &runner.InitReply{Message: "ok"})
}

func (f *fakeRunner) SelectWorkspace(_ context.Context, in *runner.WorkspaceRequest, _ ...grpc.CallOption) (*runner.WorkspaceReply, error) {
	return fakeRPC(f, "SelectWorkspace", in, &runner.WorkspaceReply{Message: "ok"})
}

func (f *fakeRunner) ForceUnlock(_ context.Context, in *runner.ForceUnlockRequest, _ ...grpc.CallOption) (*runner.ForceUnlockReply, error) {
	return fakeRPC(f, "ForceUnlock", in, &runner.ForceUnlockReply{})
}

func (f *fakeRunner) Plan(_ context.Context, in *runner.PlanRequest, _ ...grpc.CallOption) (*runner.PlanReply, error) {
	return fakeRPC(f, "Plan", in, &runner.PlanReply{Message: "ok", Drifted: true, PlanCreated: true})
}

func (f *fakeRunner) ShowPlanFileRaw(_ context.Context, in *runner.ShowPlanFileRawRequest, _ ...grpc.CallOption) (*runner.ShowPlanFileRawReply, error) {
	return fakeRPC(f, "ShowPlanFileRaw", in, &runner.ShowPlanFileRawReply{RawOutput: "fake plan"})
}

func (f *fakeRunner) ShowPlanFile(_ context.Context, in *runner.ShowPlanFileRequest, _ ...grpc.CallOption) (*runner.ShowPlanFileReply, error) {
	return fakeRPC(f, "ShowPlanFile", in, &runner.ShowPlanFileReply{JsonOutput: []byte(`{}`)})
}

func (f *fakeRunner) SaveTFPlan(_ context.Context, in *runner.SaveTFPlanRequest, _ ...grpc.CallOption) (*runner.SaveTFPlanReply, error) {
	return fakeRPC(f, "SaveTFPlan", in, &runner.SaveTFPlanReply{Message: "ok"})
}

func (f *fakeRunner) LoadTFPlan(_ context.Context, in *runner.LoadTFPlanRequest, _ ...grpc.CallOption) (*runner.LoadTFPlanReply, error) {
	return fakeRPC(f, "LoadTFPlan", in, &runner.LoadTFPlanReply{Message: "ok"})
}

func (f *fakeRunner) Apply(_ context.Context, in *runner.ApplyRequest, _ ...grpc.CallOption) (*runner.ApplyReply, error) {
	return fakeRPC(f, "Apply", in, &runner.ApplyReply{Message: "ok"})
}

func (f *fakeRunner) GetInventory(_ context.Context, in *runner.GetInventoryRequest, _ ...grpc.CallOption) (*runner.GetInventoryReply, error) {
	return fakeRPC(f, "GetInventory", in, &runner.GetInventoryReply{})
}

func (f *fakeRunner) Destroy(_ context.Context, in *runner.DestroyRequest, _ ...grpc.CallOption) (*runner.DestroyReply, error) {
	return fakeRPC(f, "Destroy", in, &runner.DestroyReply{Message: "ok"})
}

func (f *fakeRunner) Output(_ context.Context, in *runner.OutputRequest, _ ...grpc.CallOption) (*runner.OutputReply, error) {
	return fakeRPC(f, "Output", in, &runner.OutputReply{})
}

func (f *fakeRunner) WriteOutputs(_ context.Context, in *runner.WriteOutputsRequest, _ ...grpc.CallOption) (*runner.WriteOutputsReply, error) {
	return fakeRPC(f, "WriteOutputs", in, &runner.WriteOutputsReply{Message: "ok"})
}

func (f *fakeRunner) GetOutputs(_ context.Context, in *runner.GetOutputsRequest, _ ...grpc.CallOption) (*runner.GetOutputsReply, error) {
	return fakeRPC(f, "GetOutputs", in, &runner.GetOutputsReply{})
}

func (f *fakeRunner) FinalizeSecrets(_ context.Context, in *runner.FinalizeSecretsRequest, _ ...grpc.CallOption) (*runner.FinalizeSecretsReply, error) {
	return fakeRPC(f, "FinalizeSecrets", in, &runner.FinalizeSecretsReply{Message: "ok"})
}

func (f *fakeRunner) StartBreakTheGlassSession(_ context.Context, in *runner.BreakTheGlassRequest, _ ...grpc.CallOption) (*runner.BreakTheGlassReply, error) {
	return fakeRPC(f, "StartBreakTheGlassSession", in, &runner.BreakTheGlassReply{Success: true})
}

func (f *fakeRunner) HasBreakTheGlassSessionDone(_ context.Context, in *runner.BreakTheGlassRequest, _ ...grpc.CallOption) (*runner.BreakTheGlassReply, error) {
	return fakeRPC(f, "HasBreakTheGlassSessionDone", in, &runner.BreakTheGlassReply{Success: true})
}

// newFakePatcher stores obj in a fake API server (status subresource enabled)
// and returns a client for it, plus a SerialPatcher primed with the stored
// object. obj is updated in place to the stored version.
func newFakePatcher(t *testing.T, obj *infrav1.Terraform, others ...client.Object) (client.Client, *patch.SerialPatcher) {
	t.Helper()
	return newInterceptedFakePatcher(t, interceptor.Funcs{}, obj, others...)
}

// newInterceptedFakePatcher is newFakePatcher with the fake API server's
// client wrapped in funcs, so a test can observe or fail its calls.
func newInterceptedFakePatcher(t *testing.T, funcs interceptor.Funcs, obj *infrav1.Terraform, others ...client.Object) (client.Client, *patch.SerialPatcher) {
	t.Helper()
	c := fake.NewClientBuilder().
		WithScheme(k8sClient.Scheme()).
		WithStatusSubresource(&infrav1.Terraform{}).
		WithObjects(append([]client.Object{obj}, others...)...).
		WithInterceptorFuncs(funcs).
		Build()
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(obj), obj); err != nil {
		t.Fatalf("reading back %s: %v", obj.Name, err)
	}
	return c, patch.NewSerialPatcher(obj, c)
}

// newFakeReconciler returns a reconciler wired to c, with the options
// SetupWithManager would set that the step functions read.
func newFakeReconciler(c client.Client) *TerraformReconciler {
	httpClient := retryablehttp.NewClient()
	httpClient.RetryMax = 0
	httpClient.Logger = nil
	return &TerraformReconciler{
		Client:           c,
		Scheme:           c.Scheme(),
		EventRecorder:    events.NewFakeRecorder(100),
		httpClient:       httpClient,
		patchOptions:     []patch.Option{patch.WithOwnedConditions{Conditions: infrav1.OwnedConditions}, patch.WithFieldOwner("tf-controller")},
		RunnerRPCTimeout: time.Minute,
	}
}

const helloWorldTarGzPath = "data/terraform-hello-world-example.tar.gz"

// newArtifactServer serves the hello-world tarball at /hello.tar.gz.
func newArtifactServer(t *testing.T) *httptest.Server {
	t.Helper()
	helloWorld, err := os.ReadFile(helloWorldTarGzPath)
	if err != nil {
		t.Fatalf("reading %s: %v", helloWorldTarGzPath, err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hello.tar.gz" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(helloWorld)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// helloWorldSource is a GitRepository whose artifact is the hello-world
// tarball served by srv.
func helloWorldSource(srv *httptest.Server) *sourcev1.GitRepository {
	return &sourcev1.GitRepository{
		ObjectMeta: metav1.ObjectMeta{Name: "hello", Namespace: "flux-system"},
		Status: sourcev1.GitRepositoryStatus{
			Artifact: &meta.Artifact{
				Path:           "gitrepository/flux-system/hello/main.tar.gz",
				URL:            srv.URL + "/hello.tar.gz",
				Revision:       "main@sha1:b8e362c206e3d0cbb7ed22ced771a0056455a2fb",
				Digest:         "sha256:ignored",
				LastUpdateTime: metav1.Now(),
			},
		},
	}
}

// fakeTerraform is a minimal Terraform object for step-function unit tests.
func fakeTerraform(name string) *infrav1.Terraform {
	return &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  "flux-system",
			Generation: 1,
			Finalizers: []string{infrav1.TerraformFinalizer},
		},
		Spec: infrav1.TerraformSpec{
			ApprovePlan: infrav1.ApprovePlanAutoValue,
			Path:        "./terraform-hello-world-example",
			SourceRef: infrav1.CrossNamespaceSourceReference{
				Kind:      "GitRepository",
				Name:      "hello",
				Namespace: "flux-system",
			},
			Interval: metav1.Duration{Duration: time.Hour},
		},
	}
}
