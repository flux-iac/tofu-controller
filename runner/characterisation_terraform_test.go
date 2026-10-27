package runner_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"

	. "github.com/onsi/gomega"
)

// terraformSession builds a harness and a stub terraform, and starts a session
// for tf (or a plain Terraform object when tf is nil).
func terraformSession(t *testing.T, tf *infrav1.Terraform) (*harness, *stubTerraform, string) {
	t.Helper()

	if tf == nil {
		tf = &infrav1.Terraform{ObjectMeta: metav1.ObjectMeta{Name: "stub", Namespace: "default"}}
	}
	h := newHarness(t)
	stub := newStubTerraform(t)
	id := h.newSession(t, tf, stub.Path(), t.TempDir())

	return h, stub, id
}

// lockVerbs are the four calls that report a held state lock as an Aborted
// status with a StateLock detail.
var lockVerbs = []struct {
	name string
	sub  string
	call func(t *testing.T, h *harness, id string) error
}{
	{"Init", "init", func(t *testing.T, h *harness, id string) error {
		_, err := h.Client.Init(t.Context(), &runner.InitRequest{TfInstance: id})
		return err
	}},
	{"Plan", "plan", func(t *testing.T, h *harness, id string) error {
		_, err := h.Client.Plan(t.Context(), &runner.PlanRequest{TfInstance: id, Out: filepath.Join(h.Dir, "tfplan"), Refresh: true})
		return err
	}},
	{"Apply", "apply", func(t *testing.T, h *harness, id string) error {
		_, err := h.Client.Apply(t.Context(), &runner.ApplyRequest{TfInstance: id})
		return err
	}},
	{"Destroy", "destroy", func(t *testing.T, h *harness, id string) error {
		_, err := h.Client.Destroy(t.Context(), &runner.DestroyRequest{TfInstance: id})
		return err
	}},
}

// stateLockDetail returns the StateLock detail attached to err, if any.
func stateLockDetail(err error) (*runner.StateLock, bool) {
	for _, d := range status.Convert(err).Details() {
		if lock, ok := d.(*runner.StateLock); ok {
			return lock, true
		}
	}

	return nil, false
}

func TestCharacterisation_LockDetail(t *testing.T) {
	for _, v := range lockVerbs {
		t.Run(v.name, func(t *testing.T) {
			g := NewWithT(t)
			h, stub, id := terraformSession(t, nil)
			stub.On(v.sub, "", lockHeldStderr, 1)

			err := v.call(t, h, id)

			g.Expect(status.Code(err)).To(Equal(codes.Aborted))
			g.Expect(status.Convert(err).Message()).To(HavePrefix("error acquiring the state lock: Lock Info:"))
			g.Expect(status.Convert(err).Message()).To(ContainSubstring("9db590f1-b6fe-c5f2-2678-8804f089deba"))
			g.Expect(status.Convert(err).Details()).To(HaveLen(1))
			lock, ok := stateLockDetail(err)
			g.Expect(ok).To(BeTrue(), "the StateLock detail is attached")
			g.Expect(lock.GetId()).To(Equal("9db590f1-b6fe-c5f2-2678-8804f089deba"))
		})
	}
}

// When the lock cannot be acquired but the output has no "Lock Info" block,
// there is no lock to report, so the call fails with terraform's own error
// and no lock detail.
func TestCharacterisation_LockWithoutInfo(t *testing.T) {
	for _, v := range lockVerbs {
		t.Run(v.name, func(t *testing.T) {
			g := NewWithT(t)
			h, stub, id := terraformSession(t, nil)
			stub.On(v.sub, "", lockAcquireFailedStderr, 1)

			err := v.call(t, h, id)

			g.Expect(status.Code(err)).To(Equal(codes.Internal))
			g.Expect(status.Convert(err).Message()).To(ContainSubstring("permission denied"))
			g.Expect(status.Convert(err).Details()).To(BeEmpty())
		})
	}
}

// ForceUnlock on state that is not locked fails with FailedPrecondition and a
// StateLock detail naming the requested lock. The reply is lost on the wire.
func TestCharacterisation_ForceUnlockNotLocked(t *testing.T) {
	g := NewWithT(t)
	h, stub, id := terraformSession(t, nil)
	stub.On("force-unlock", "", notLockedStderr, 1)

	reply, err := h.Client.ForceUnlock(t.Context(), &runner.ForceUnlockRequest{TfInstance: id, LockIdentifier: "abc"})

	g.Expect(reply).To(BeNil())
	g.Expect(status.Code(err)).To(Equal(codes.FailedPrecondition))
	g.Expect(status.Convert(err).Message()).To(ContainSubstring("LocalState not locked"))
	details := status.Convert(err).Details()
	g.Expect(details).To(HaveLen(1))
	lock, ok := details[0].(*runner.StateLock)
	g.Expect(ok).To(BeTrue(), "the detail is a StateLock")
	g.Expect(lock.GetId()).To(Equal("abc"))
}

// The kubernetes backend, the controller's default, words "not locked" its own
// way. ForceUnlock reports it the same as any other backend.
func TestCharacterisation_ForceUnlockAlreadyUnlocked(t *testing.T) {
	g := NewWithT(t)
	h, stub, id := terraformSession(t, nil)
	stub.On("force-unlock", "", alreadyUnlockedStderr, 1)

	_, err := h.Client.ForceUnlock(t.Context(), &runner.ForceUnlockRequest{TfInstance: id, LockIdentifier: "abc"})

	g.Expect(status.Code(err)).To(Equal(codes.FailedPrecondition))
	g.Expect(status.Convert(err).Message()).To(ContainSubstring("state is already unlocked"))
	lock, ok := stateLockDetail(err)
	g.Expect(ok).To(BeTrue(), "the StateLock detail is attached")
	g.Expect(lock.GetId()).To(Equal("abc"))
}

// When another holder has taken the lock, ForceUnlock reports that lock as
// held, with the new holder's ID, so the controller can ask about it.
func TestCharacterisation_ForceUnlockLockedByAnother(t *testing.T) {
	g := NewWithT(t)
	h, stub, id := terraformSession(t, nil)
	stub.On("force-unlock", "", lockMismatchStderr, 1)

	_, err := h.Client.ForceUnlock(t.Context(), &runner.ForceUnlockRequest{TfInstance: id, LockIdentifier: "abc"})

	g.Expect(status.Code(err)).To(Equal(codes.Aborted))
	lock, ok := stateLockDetail(err)
	g.Expect(ok).To(BeTrue(), "the StateLock detail is attached")
	g.Expect(lock.GetId()).To(Equal("5b4e9f2a-1c3d-4e5f-8a9b-0c1d2e3f4a5b"))
}

// A plan that fails variable validation has the offending variable value
// masked before the error travels back to the controller.
func TestCharacterisation_PlanValidationLeak(t *testing.T) {
	g := NewWithT(t)
	h, stub, id := terraformSession(t, nil)
	stub.On("plan", "", validationFailedStderr, 1)

	_, err := h.Client.Plan(t.Context(), &runner.PlanRequest{TfInstance: id, Out: filepath.Join(h.Dir, "tfplan"), Refresh: true})

	g.Expect(status.Code(err)).To(Equal(codes.Internal))
	g.Expect(status.Convert(err).Message()).NotTo(ContainSubstring("SUPERSECRET"))
	g.Expect(status.Convert(err).Message()).To(ContainSubstring("var.s is ***"))
}

// When selecting the workspace fails and so does creating it, the message
// names the workspace and keeps the reason terraform gave for each.
func TestCharacterisation_SelectWorkspaceFailure(t *testing.T) {
	g := NewWithT(t)
	tf := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{Name: "stub", Namespace: "default"},
		Spec:       infrav1.TerraformSpec{Workspace: "prod"},
	}
	h, stub, id := terraformSession(t, tf)
	// "workspace select" fails because the workspace is missing, and
	// "workspace new" fails for its own reason. The failed "new" is logged,
	// and the call reports both.
	stub.On("workspace_select", "", `Workspace "prod" doesn't exist.`, 1)
	stub.On("workspace_new", "", "backend unreachable", 1)

	_, err := h.Client.SelectWorkspace(t.Context(), &runner.WorkspaceRequest{TfInstance: id})

	g.Expect(status.Code(err)).To(Equal(codes.Internal))
	msg := status.Convert(err).Message()
	g.Expect(msg).To(HavePrefix("failed to select workspace prod: "))
	g.Expect(msg).To(ContainSubstring(`Workspace "prod" doesn't exist.`))
	g.Expect(msg).To(ContainSubstring("backend unreachable"))
	// The failed "workspace new" is logged as an error.
	g.Expect(h.Logs.String()).To(MatchRegexp(`"msg"="failed to create workspace" "error"="[^"]*backend unreachable`))
}

// An existing workspace is selected without trying to create it. A missing
// one is created and then selected. Neither is an error worth logging.
func TestCharacterisation_SelectWorkspace(t *testing.T) {
	prod := func() *infrav1.Terraform {
		return &infrav1.Terraform{
			ObjectMeta: metav1.ObjectMeta{Name: "stub", Namespace: "default"},
			Spec:       infrav1.TerraformSpec{Workspace: "prod"},
		}
	}
	workspaceCalls := func(stub *stubTerraform) []string {
		var got []string
		for _, call := range stub.Calls() {
			if call[0] == "workspace" {
				got = append(got, call[1])
			}
		}
		return got
	}

	t.Run("existing", func(t *testing.T) {
		g := NewWithT(t)
		h, stub, id := terraformSession(t, prod())

		_, err := h.Client.SelectWorkspace(t.Context(), &runner.WorkspaceRequest{TfInstance: id})

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(workspaceCalls(stub)).To(Equal([]string{"select"}))
		g.Expect(h.Logs.String()).NotTo(ContainSubstring(`"error"=`))
	})

	t.Run("missing", func(t *testing.T) {
		g := NewWithT(t)
		h, stub, id := terraformSession(t, prod())
		stub.OnExits("workspace_select", "", `Workspace "prod" doesn't exist.`, 1, 0)

		_, err := h.Client.SelectWorkspace(t.Context(), &runner.WorkspaceRequest{TfInstance: id})

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(workspaceCalls(stub)).To(Equal([]string{"select", "new", "select"}))
		g.Expect(h.Logs.String()).NotTo(ContainSubstring(`"error"=`))
	})
}

// A failing init with nothing on stderr still reports terraform's exit status.
func TestCharacterisation_InitEmptyStderr(t *testing.T) {
	g := NewWithT(t)
	h, stub, id := terraformSession(t, nil)
	stub.On("init", "", "", 1)

	_, err := h.Client.Init(t.Context(), &runner.InitRequest{TfInstance: id})

	g.Expect(status.Code(err)).To(Equal(codes.Internal))
	g.Expect(status.Convert(err).Message()).NotTo(BeEmpty())
	g.Expect(status.Convert(err).Message()).To(ContainSubstring("exit status 1"))
}

// Stopping the server cancels the running command, and the
// caller is told the command was interrupted.
func TestCharacterisation_ApplyInterrupted(t *testing.T) {
	g := NewWithT(t)
	h, stub, id := terraformSession(t, nil)
	stub.Sleep("apply", 30)

	errCh := make(chan error, 1)
	go func() {
		_, err := h.Client.Apply(t.Context(), &runner.ApplyRequest{TfInstance: id})
		errCh <- err
	}()

	g.Eventually(func() bool { return stub.Sleeping("apply") }, 10*time.Second, 20*time.Millisecond).
		Should(BeTrue(), "apply is sleeping with its interrupt handler installed")
	h.Stop()

	var err error
	g.Eventually(errCh, 10*time.Second).Should(Receive(&err))
	g.Expect(status.Code(err)).To(Equal(codes.Canceled))
	g.Expect(status.Convert(err).Message()).To(ContainSubstring("terraform interrupted"))
}

// A tfvars path that goes through a regular file, such as "vars.tfvars/x",
// makes os.Stat fail with something other than "not exist". That is bad
// input, reported as such.
func TestCharacterisation_PlanTfVarsPathThroughFile(t *testing.T) {
	g := NewWithT(t)
	root := t.TempDir()
	g.Expect(os.WriteFile(filepath.Join(root, "vars.tfvars"), []byte("a = 1"), 0o600)).To(Succeed())

	var err error
	g.Expect(func() {
		_, err = runner.TfVarsFileOptions(root, []string{"vars.tfvars/x"})
	}).NotTo(Panic())

	var invalid *runner.InvalidInputError
	g.Expect(errors.As(err, &invalid)).To(BeTrue(), "the error is an InvalidInputError, got %T", err)
	g.Expect(err.Error()).To(ContainSubstring("not a directory"))
}

// Pins what the tfvars helper returns today. The join error branch cannot be
// reached: securejoin only fails on an error from the file system while
// resolving symlinks, which a path that does not exist never causes.
func TestCharacterisation_TfVarsFileOptions(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"a.tfvars", "b.tfvars"} {
		if err := os.WriteFile(filepath.Join(root, f), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("missing file", func(t *testing.T) {
		g := NewWithT(t)
		opts, err := runner.TfVarsFileOptions(root, []string{"nope.tfvars"})
		g.Expect(opts).To(BeNil())
		g.Expect(err).To(MatchError("error running plan: tfvars file's path does not exist: " + filepath.Join(root, "nope.tfvars")))
	})

	t.Run("directory", func(t *testing.T) {
		g := NewWithT(t)
		opts, err := runner.TfVarsFileOptions(root, []string{"dir"})
		g.Expect(opts).To(BeNil())
		g.Expect(err).To(MatchError("error running Plan: tfvars file's path is a directory: " + filepath.Join(root, "dir")))
	})

	t.Run("two files in order", func(t *testing.T) {
		g := NewWithT(t)
		opts, err := runner.TfVarsFileOptions(root, []string{"a.tfvars", "b.tfvars"})
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(opts).To(HaveLen(2))
	})
}

// With the backend disabled the plan is a dummy and there is no plan file, so
// no readable plan is stored and terraform is never asked to show one.
func TestCharacterisation_SaveTFPlanReadableWithBackendDisabled(t *testing.T) {
	for _, format := range []string{"json", "human"} {
		t.Run(format, func(t *testing.T) {
			g := NewWithT(t)
			tf := &infrav1.Terraform{
				ObjectMeta: metav1.ObjectMeta{Name: "stub", Namespace: "default"},
				Spec:       infrav1.TerraformSpec{StoreReadablePlan: format},
			}
			h, stub, id := terraformSession(t, tf)
			stub.On("show", "", "no plan file", 1)

			reply, err := h.Client.SaveTFPlan(t.Context(), &runner.SaveTFPlanRequest{
				TfInstance:               id,
				BackendCompletelyDisable: true,
				Name:                     "stub",
				Namespace:                "default",
				Uuid:                     "uuid",
				Revision:                 "main@sha1:0123456789abcdef",
			})

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(reply.Message).To(Equal("ok"))
			for _, call := range stub.Calls() {
				g.Expect(call[0]).NotTo(Equal("show"))
			}
		})
	}
}

// With human readable plan logging on and no plan given, apply first shows the
// default plan file. When that fails, the failure is logged and apply still
// runs.
func TestCharacterisation_ApplyHumanReadableMissingPlan(t *testing.T) {
	g := NewWithT(t)
	t.Setenv("LOG_HUMAN_READABLE_PLAN", "1")
	h, stub, id := terraformSession(t, nil)
	stub.On("show", "", "no plan file", 1)

	reply, err := h.Client.Apply(t.Context(), &runner.ApplyRequest{TfInstance: id, DirOrPlan: ""})

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(reply.Message).To(Equal("ok"))
	var subs []string
	for _, call := range stub.Calls() {
		subs = append(subs, call[0])
	}
	g.Expect(subs).To(ContainElement("show"))
	g.Expect(subs).To(ContainElement("apply"))
	g.Expect(h.Logs.String()).To(MatchRegexp(`"msg"="failed to print the human readable plan" "error"="[^"]*no plan file`))
}

// A requested backend config key that is missing from the Secret is not
// passed to init, as with a ConfigMap, so it cannot blank the setting.
func TestCharacterisation_BackendConfigMissingKey(t *testing.T) {
	g := NewWithT(t)
	tf := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{Name: "stub", Namespace: "default"},
		Spec: infrav1.TerraformSpec{
			BackendConfigsFrom: []infrav1.BackendConfigsReference{
				{Kind: "Secret", Name: "backend", Keys: []string{"path", "bucket"}},
			},
		},
	}
	h := newHarness(t, secret("backend", map[string]string{"bucket": "b"}))
	stub := newStubTerraform(t)
	id := h.newSession(t, tf, stub.Path(), t.TempDir())

	_, err := h.Client.Init(t.Context(), &runner.InitRequest{TfInstance: id})

	g.Expect(err).NotTo(HaveOccurred())
	var initArgs []string
	for _, call := range stub.Calls() {
		if call[0] == "init" {
			initArgs = call
		}
	}
	g.Expect(initArgs).NotTo(BeEmpty())
	g.Expect(initArgs).NotTo(ContainElement(HavePrefix("-backend-config=path=")))
	g.Expect(initArgs).To(ContainElement("-backend-config=bucket=b"))
}

// When terraform rejects one of the variables SetEnv was given, only the
// error is logged, never the values. TF_LOG is not allowed to be set, so it
// triggers that.
func TestCharacterisation_SetEnvDoesNotLogValues(t *testing.T) {
	g := NewWithT(t)
	h, _, id := terraformSession(t, nil)

	_, err := h.Client.SetEnv(t.Context(), &runner.SetEnvRequest{
		TfInstance: id,
		Envs:       map[string]string{"TF_LOG": "x", "SECRET": "hunter2"},
	})

	g.Expect(err).To(HaveOccurred())
	g.Expect(status.Convert(err).Message()).To(ContainSubstring("TF_LOG"))
	g.Expect(h.Logs.String()).NotTo(ContainSubstring("hunter2"))
	g.Expect(strings.Count(h.Logs.String(), "rpc failed")).To(Equal(1))
}

// NewTerraform gives terraform the Terraform's runner termination grace
// period to exit after an interrupt.
func TestCharacterisation_NewTerraformWaitDelay(t *testing.T) {
	g := NewWithT(t)
	tf := &infrav1.Terraform{
		ObjectMeta: metav1.ObjectMeta{Name: "stub", Namespace: "default"},
		Spec:       infrav1.TerraformSpec{RunnerTerminationGracePeriodSeconds: ptr.To(int64(300))},
	}

	h, _, _ := terraformSession(t, tf)

	g.Expect(runner.SessionWaitDelay(h.Server)).To(Equal(300 * time.Second))
}

// captureStderr returns what fn writes to the process's stderr. It swaps the
// process-wide os.Stderr, so tests that use it must not run in parallel.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	out := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		out <- string(b)
	}()

	fn()
	_ = w.Close()

	return <-out
}

// captureStdout returns what fn writes to the process's stdout. It swaps the
// process-wide os.Stdout, so tests that use it must not run in parallel.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	out := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		out <- string(b)
	}()

	fn()
	_ = w.Close()

	return <-out
}

// Init and plan print terraform's stderr, with variable values masked, only
// when they fail for a reason other than a held state lock, and never when
// terraform output is disabled.
func TestCharacterisation_InitPlanStderrEcho(t *testing.T) {
	verbs := map[string]func(t *testing.T, h *harness, id string) error{
		"init": func(t *testing.T, h *harness, id string) error {
			_, err := h.Client.Init(t.Context(), &runner.InitRequest{TfInstance: id})
			return err
		},
		"plan": func(t *testing.T, h *harness, id string) error {
			_, err := h.Client.Plan(t.Context(), &runner.PlanRequest{TfInstance: id, Refresh: true})
			return err
		},
	}
	cases := []struct {
		name      string
		stderr    string
		exit      int
		quiet     bool
		wantPrint bool
	}{
		{name: "success", stderr: "Warning: something to note\n", exit: 0},
		{name: "lock held", stderr: lockHeldStderr, exit: 1},
		{name: "failure", stderr: validationFailedStderr, exit: 1, wantPrint: true},
		{name: "failure with output disabled", stderr: validationFailedStderr, exit: 1, quiet: true},
	}

	for sub, call := range verbs {
		for _, c := range cases {
			t.Run(sub+"/"+c.name, func(t *testing.T) {
				g := NewWithT(t)
				if c.quiet {
					t.Setenv("DISABLE_TF_LOGS", "1")
				} else {
					t.Setenv("DISABLE_TF_LOGS", "")
				}
				h, stub, id := terraformSession(t, nil)
				stub.On(sub, "", c.stderr, c.exit)

				var err error
				printed := captureStderr(t, func() { err = call(t, h, id) })

				g.Expect(err != nil).To(Equal(c.exit != 0))
				if c.wantPrint {
					g.Expect(printed).To(ContainSubstring("var.s is ***"))
					g.Expect(printed).NotTo(ContainSubstring("SUPERSECRET"))
				} else {
					g.Expect(printed).To(BeEmpty())
				}
			})
		}
	}
}

// Apply and destroy stream terraform's stderr to the pod as it is written,
// with variable values masked.
func TestCharacterisation_ApplyDestroyStderrIsSanitised(t *testing.T) {
	const tfvarsStderr = `
Error: Invalid value for input variable

  on generated.auto.tfvars.json line 1:
   1: {"s":"SUPERSECRET"}
`
	verbs := map[string]func(t *testing.T, h *harness, id string) error{
		"apply": func(t *testing.T, h *harness, id string) error {
			_, err := h.Client.Apply(t.Context(), &runner.ApplyRequest{TfInstance: id})
			return err
		},
		"destroy": func(t *testing.T, h *harness, id string) error {
			_, err := h.Client.Destroy(t.Context(), &runner.DestroyRequest{TfInstance: id})
			return err
		},
	}
	outputs := map[string]struct {
		stderr string
		masked string
	}{
		"validation failure": {validationFailedStderr, "var.s is ***"},
		"tfvars diagnostic":  {tfvarsStderr, `1: {"s":"***"}`},
	}

	for sub, call := range verbs {
		for name, out := range outputs {
			t.Run(sub+"/"+name, func(t *testing.T) {
				g := NewWithT(t)
				t.Setenv("DISABLE_TF_LOGS", "")
				h, stub, id := terraformSession(t, nil)
				stub.On(sub, "", out.stderr, 1)

				var err error
				printed := captureStderr(t, func() { err = call(t, h, id) })

				g.Expect(err).To(HaveOccurred())
				g.Expect(printed).To(ContainSubstring(out.masked))
				g.Expect(printed).NotTo(ContainSubstring("SUPERSECRET"))
			})
		}
	}
}

// Apply and destroy stream terraform's stdout to the pod with variable values
// masked too, because warnings such as failed check assertions go to stdout.
func TestCharacterisation_ApplyDestroyStdoutIsSanitised(t *testing.T) {
	const checkWarningStdout = `
Warning: Check block assertion failed

  on main.tf line 9, in check "pw":
   9:     condition = length(var.pw) > 10
    ├────────────────
    │ var.pw is "hunter2"

Password too short.

Apply complete! Resources: 0 added, 0 changed, 0 destroyed.
`
	verbs := map[string]func(t *testing.T, h *harness, id string) error{
		"apply": func(t *testing.T, h *harness, id string) error {
			_, err := h.Client.Apply(t.Context(), &runner.ApplyRequest{TfInstance: id})
			return err
		},
		"destroy": func(t *testing.T, h *harness, id string) error {
			_, err := h.Client.Destroy(t.Context(), &runner.DestroyRequest{TfInstance: id})
			return err
		},
	}

	for sub, call := range verbs {
		t.Run(sub, func(t *testing.T) {
			g := NewWithT(t)
			t.Setenv("DISABLE_TF_LOGS", "")
			h, stub, id := terraformSession(t, nil)
			stub.On(sub, checkWarningStdout, "", 0)

			var err error
			printed := captureStdout(t, func() { err = call(t, h, id) })

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(printed).To(ContainSubstring("│ var.pw is ***"))
			g.Expect(printed).To(ContainSubstring("Apply complete!"))
			g.Expect(printed).NotTo(ContainSubstring("hunter2"))
		})
	}
}
