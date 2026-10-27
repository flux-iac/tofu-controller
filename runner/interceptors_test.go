package runner_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/flux-iac/tofu-controller/runner"

	. "github.com/onsi/gomega"
)

func TestInterceptors(t *testing.T) {
	t.Run("RecoversPanic", func(t *testing.T) {
		g := NewWithT(t)
		h := newHarnessWith(t, []harnessOption{withInterceptors(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if key.Name == "boom" {
					panic("kaboom")
				}
				return c.Get(ctx, key, obj, opts...)
			},
		})}, secret("outputs", map[string]string{"a": "1"}))

		_, err := h.Client.GetOutputs(t.Context(), &runner.GetOutputsRequest{Namespace: "default", SecretName: "boom"})

		g.Expect(status.Code(err)).To(Equal(codes.Internal))
		g.Expect(status.Convert(err).Message()).To(Equal("runner panic in /runner.Runner/GetOutputs: kaboom"))
		g.Expect(h.Logs.String()).To(ContainSubstring("kaboom"))

		reply, err := h.Client.GetOutputs(t.Context(), &runner.GetOutputsRequest{Namespace: "default", SecretName: "outputs"})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(reply.Outputs).To(Equal(map[string]string{"a": "1"}))
	})

	t.Run("SessionMismatch", func(t *testing.T) {
		g := NewWithT(t)
		h := newHarness(t)
		a := h.newSession(t, newTerraform("a"), newStubTerraform(t).Path(), t.TempDir())

		_, err := h.Client.Plan(t.Context(), &runner.PlanRequest{TfInstance: "b"})

		g.Expect(status.Code(err)).To(Equal(codes.FailedPrecondition))
		g.Expect(status.Convert(err).Message()).To(Equal(
			"terraform session mismatch: requested instance id 'b' does not match the current instance '" + a + "'"))
	})

	t.Run("FailedNewTerraformKeepsSession", func(t *testing.T) {
		g := NewWithT(t)
		h := newHarness(t)
		a := h.newSession(t, newTerraform("a"), newStubTerraform(t).Path(), t.TempDir())

		failures := map[string]*runner.NewTerraformRequest{
			"no exec path": {
				WorkingDir: t.TempDir(),
				ExecPath:   "",
				InstanceID: "b",
			},
			"undecodable object": {
				WorkingDir: t.TempDir(),
				ExecPath:   newStubTerraform(t).Path(),
				Terraform:  []byte("not a terraform object"),
				InstanceID: "b",
			},
		}
		for name, req := range failures {
			_, err := h.Client.NewTerraform(t.Context(), req)
			g.Expect(err).To(HaveOccurred(), name)

			_, err = h.Client.SetEnv(t.Context(), &runner.SetEnvRequest{TfInstance: a})
			g.Expect(err).NotTo(HaveOccurred(), name)
			_, err = h.Client.SetEnv(t.Context(), &runner.SetEnvRequest{TfInstance: "b"})
			g.Expect(status.Code(err)).To(Equal(codes.FailedPrecondition), name)
		}
	})

	t.Run("TwoObjectsOneServer", func(t *testing.T) {
		g := NewWithT(t)
		h := newHarness(t)
		stub := newStubTerraform(t)
		a := h.newSession(t, newTerraform("a"), stub.Path(), t.TempDir())
		b := h.newSession(t, newTerraform("b"), stub.Path(), t.TempDir())

		_, err := h.Client.Output(t.Context(), &runner.OutputRequest{TfInstance: a})

		g.Expect(status.Code(err)).To(Equal(codes.FailedPrecondition))
		g.Expect(status.Convert(err).Message()).To(Equal(
			"terraform session mismatch: requested instance id '" + a + "' does not match the current instance '" + b + "'"))
	})

	t.Run("LogsOneFailureWithRPCAndInstance", func(t *testing.T) {
		g := NewWithT(t)
		h := newHarness(t)
		a := h.newSession(t, newTerraform("a"), newStubTerraform(t).Path(), t.TempDir())

		_, err := h.Client.WriteBackendConfig(t.Context(), &runner.WriteBackendConfigRequest{DirPath: filepath.Join(h.Dir, "missing")})
		g.Expect(err).To(HaveOccurred())
		_, err = h.Client.SetEnv(t.Context(), &runner.SetEnvRequest{TfInstance: a, Envs: map[string]string{"TF_LOG": "x"}})
		g.Expect(err).To(HaveOccurred())

		logs := h.Logs.String()
		g.Expect(strings.Count(logs, `"msg"="rpc failed"`)).To(Equal(2))
		// The logged error is the one the handler returned, before it is
		// mapped to a gRPC status.
		g.Expect(logs).To(MatchRegexp(`(?m)"msg"="rpc failed" "error"="open [^"]*/missing/backend_override.tf: no such file or directory" "rpc"="WriteBackendConfig"$`))
		g.Expect(logs).To(MatchRegexp(`(?m)"msg"="rpc failed" "error"="manual setting of env var \\"TF_LOG\\" detected" "rpc"="SetEnv" "instance-id"="` + a + `"$`))
		g.Expect(logs).NotTo(ContainSubstring("rpc error: code ="))
	})

	t.Run("LogsLockErrorWithLockID", func(t *testing.T) {
		g := NewWithT(t)
		h := newHarness(t)
		stub := newStubTerraform(t)
		a := h.newSession(t, newTerraform("a"), stub.Path(), t.TempDir())
		stub.On("plan", "", lockHeldStderr, 1)

		_, err := h.Client.Plan(t.Context(), &runner.PlanRequest{TfInstance: a, Refresh: true})
		g.Expect(status.Code(err)).To(Equal(codes.Aborted))

		logs := h.Logs.String()
		g.Expect(strings.Count(logs, `"msg"="rpc failed"`)).To(Equal(1))
		g.Expect(logs).To(MatchRegexp(`"msg"="rpc failed" "error"="error acquiring the state lock: Lock Info:[^"]*9db590f1-b6fe-c5f2-2678-8804f089deba`))
	})
}
