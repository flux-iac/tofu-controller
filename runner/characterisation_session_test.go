package runner_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"

	. "github.com/onsi/gomega"
)

// Calls every RPC that takes part in the session check with an instance id
// that was never created. A fresh runner has no session, so the validating
// RPCs fail as a failed precondition with the "session error" text; the others
// ignore the id and do their own work.
func TestCharacterisation_SessionValidation(t *testing.T) {
	const nope = "nope"

	rpcs := []struct {
		name      string
		validates bool
		call      func(ctx context.Context, t *testing.T, h *harness) error
	}{
		{"SetEnv", true, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.SetEnv(ctx, &runner.SetEnvRequest{TfInstance: nope})
			return err
		}},
		{"Init", true, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.Init(ctx, &runner.InitRequest{TfInstance: nope})
			return err
		}},
		{"SelectWorkspace", true, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.SelectWorkspace(ctx, &runner.WorkspaceRequest{TfInstance: nope})
			return err
		}},
		{"Plan", true, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.Plan(ctx, &runner.PlanRequest{TfInstance: nope})
			return err
		}},
		{"ShowPlanFile", true, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.ShowPlanFile(ctx, &runner.ShowPlanFileRequest{TfInstance: nope})
			return err
		}},
		{"ShowPlanFileRaw", true, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.ShowPlanFileRaw(ctx, &runner.ShowPlanFileRawRequest{TfInstance: nope})
			return err
		}},
		{"SaveTFPlan", true, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.SaveTFPlan(ctx, &runner.SaveTFPlanRequest{TfInstance: nope})
			return err
		}},
		{"LoadTFPlan", true, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.LoadTFPlan(ctx, &runner.LoadTFPlanRequest{TfInstance: nope})
			return err
		}},
		{"Apply", true, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.Apply(ctx, &runner.ApplyRequest{TfInstance: nope})
			return err
		}},
		{"Destroy", true, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.Destroy(ctx, &runner.DestroyRequest{TfInstance: nope})
			return err
		}},
		{"Output", true, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.Output(ctx, &runner.OutputRequest{TfInstance: nope})
			return err
		}},
		{"GetInventory", true, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.GetInventory(ctx, &runner.GetInventoryRequest{TfInstance: nope})
			return err
		}},
		{"GenerateVarsForTF", true, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.GenerateVarsForTF(ctx, &runner.GenerateVarsForTFRequest{TfInstance: nope})
			return err
		}},
		{"GenerateTemplate", true, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.GenerateTemplate(ctx, &runner.GenerateTemplateRequest{TfInstance: nope, WorkingDir: t.TempDir()})
			return err
		}},
		{"ForceUnlock", true, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.ForceUnlock(ctx, &runner.ForceUnlockRequest{TfInstance: nope, LockIdentifier: "abc"})
			return err
		}},

		{"LookPath", false, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.LookPath(ctx, &runner.LookPathRequest{Files: []string{"sh"}})
			return err
		}},
		{"NewTerraform", false, func(ctx context.Context, t *testing.T, h *harness) error {
			// Creating a session ignores the id it is given: it adopts it.
			_, err := h.Client.NewTerraform(ctx, &runner.NewTerraformRequest{
				WorkingDir: h.Dir,
				ExecPath:   newStubTerraform(t).Path(),
				InstanceID: nope,
			})
			return err
		}},
		{"CreateFileMappings", false, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.CreateFileMappings(ctx, &runner.CreateFileMappingsRequest{})
			return err
		}},
		{"UploadAndExtract", false, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.UploadAndExtract(ctx, &runner.UploadAndExtractRequest{Namespace: "ns", Name: "n"})
			return err
		}},
		{"CleanupDir", false, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.CleanupDir(ctx, &runner.CleanupDirRequest{TmpDir: h.Dir + "/gone"})
			return err
		}},
		{"WriteBackendConfig", false, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.WriteBackendConfig(ctx, &runner.WriteBackendConfigRequest{DirPath: h.Dir})
			return err
		}},
		{"ProcessCliConfig", false, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.ProcessCliConfig(ctx, &runner.ProcessCliConfigRequest{Namespace: "ns", Name: "missing"})
			return err
		}},
		{"WriteOutputs", false, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.WriteOutputs(ctx, &runner.WriteOutputsRequest{Namespace: "ns", SecretName: "s"})
			return err
		}},
		{"GetOutputs", false, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.GetOutputs(ctx, &runner.GetOutputsRequest{Namespace: "ns", SecretName: "missing"})
			return err
		}},
		{"FinalizeSecrets", false, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.FinalizeSecrets(ctx, &runner.FinalizeSecretsRequest{Namespace: "ns", Name: "n"})
			return err
		}},
		{"StartBreakTheGlassSession", false, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.StartBreakTheGlassSession(ctx, &runner.BreakTheGlassRequest{})
			return err
		}},
		{"HasBreakTheGlassSessionDone", false, func(ctx context.Context, t *testing.T, h *harness) error {
			_, err := h.Client.HasBreakTheGlassSessionDone(ctx, &runner.BreakTheGlassRequest{})
			return err
		}},
	}

	for _, rpc := range rpcs {
		t.Run(rpc.name, func(t *testing.T) {
			g := NewWithT(t)
			h := newHarness(t)

			err := rpc.call(t.Context(), t, h)

			if rpc.validates {
				g.Expect(err).To(HaveOccurred())
				g.Expect(status.Code(err)).To(Equal(codes.FailedPrecondition))
				g.Expect(err.Error()).To(ContainSubstring("terraform session error"))
				return
			}
			if err != nil {
				g.Expect(err.Error()).NotTo(ContainSubstring("terraform session"))
			}
		})
	}
}

// With a session in place, a different instance id gets the "mismatch" text.
func TestCharacterisation_SessionMismatch(t *testing.T) {
	g := NewWithT(t)
	h := newHarness(t)
	stub := newStubTerraform(t)
	// Starting a session gives the runner an instance id to mismatch against.
	id := h.newSession(t, newTerraform("stub"), stub.Path(), t.TempDir())

	_, err := h.Client.SetEnv(t.Context(), &runner.SetEnvRequest{TfInstance: "nope"})

	g.Expect(status.Code(err)).To(Equal(codes.FailedPrecondition))
	g.Expect(status.Convert(err).Message()).To(Equal(
		"terraform session mismatch: requested instance id 'nope' does not match the current instance '" + id + "'"))
}

// newTerraform returns a minimal Terraform object in the default namespace.
func newTerraform(name string) *infrav1.Terraform {
	return &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
	}
}
