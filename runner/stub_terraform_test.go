package runner_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"

	. "github.com/onsi/gomega"
)

// stubScript stands in for the terraform binary. For each subcommand it
// replays canned stdout and stderr, optionally sleeps, then exits with the
// canned code. Every invocation's argv is appended to the calls file.
// Output configured for "<sub>_<second word>", such as "workspace_select",
// takes precedence over "<sub>". An exit file may list several codes; each
// call uses the next one and the last one repeats.
const stubScript = `#!/bin/sh
dir=$(dirname "$0")
{
  for a in "$@"; do printf '%s\0' "$a"; done
  printf '\036'
} >> "$dir/calls"
sub=$1
key=$sub
if [ -n "$2" ] && { [ -f "$dir/${1}_$2.exit" ] || [ -f "$dir/${1}_$2.stderr" ]; }; then
  key="${1}_$2"
fi

if [ "$sub" = "version" ]; then
  echo '{"terraform_version":"1.12.5","platform":"linux_amd64","provider_selections":{},"terraform_outdated":false}'
  exit 0
fi

if [ "$sub" = "plan" ]; then
  for arg in "$@"; do
    case "$arg" in
      -out=*) : > "${arg#-out=}" ;;
    esac
  done
fi

[ -f "$dir/$key.stdout" ] && cat "$dir/$key.stdout"
[ -f "$dir/$key.stderr" ] && cat "$dir/$key.stderr" >&2

if [ -f "$dir/$sub.sleep" ]; then
  child=
  trap 'kill ${child:-} 2>/dev/null; exit 1' INT
  sleep "$(cat "$dir/$sub.sleep")" &
  child=$!
  : > "$dir/$sub.sleeping"
  wait $child
fi

if [ -f "$dir/$key.exit" ]; then
  set -- $(cat "$dir/$key.exit")
  code=$1
  if [ $# -gt 1 ]; then
    shift
    echo "$*" > "$dir/$key.exit"
  fi
  exit "$code"
fi
exit 0
`

// stubTerraform is a fake terraform binary whose output per subcommand is
// configured by the test.
type stubTerraform struct {
	t   *testing.T
	dir string
}

func newStubTerraform(t *testing.T) *stubTerraform {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "terraform"), []byte(stubScript), 0o755); err != nil {
		t.Fatal(err)
	}

	return &stubTerraform{t: t, dir: dir}
}

// Path is the location of the executable stub.
func (s *stubTerraform) Path() string {
	return filepath.Join(s.dir, "terraform")
}

// On sets what the subcommand prints and the exit code it returns.
func (s *stubTerraform) On(sub string, stdout, stderr string, exit int) {
	s.t.Helper()

	s.write(sub+".stdout", stdout)
	s.write(sub+".stderr", stderr)
	s.write(sub+".exit", fmt.Sprint(exit))
}

// OnExits is On for a subcommand that exits with each code in turn, repeating
// the last one.
func (s *stubTerraform) OnExits(sub string, stdout, stderr string, exits ...int) {
	s.t.Helper()

	codes := make([]string, len(exits))
	for i, e := range exits {
		codes[i] = fmt.Sprint(e)
	}
	s.write(sub+".stdout", stdout)
	s.write(sub+".stderr", stderr)
	s.write(sub+".exit", strings.Join(codes, " "))
}

// Sleep makes the subcommand sleep after printing, until it is interrupted.
func (s *stubTerraform) Sleep(sub string, seconds int) {
	s.t.Helper()

	s.write(sub+".sleep", fmt.Sprint(seconds))
}

// Calls returns the full argv of every invocation so far, one slice each.
func (s *stubTerraform) Calls() [][]string {
	s.t.Helper()

	b, err := os.ReadFile(filepath.Join(s.dir, "calls"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		s.t.Fatal(err)
	}

	var calls [][]string
	for _, record := range strings.Split(string(b), "\x1e") {
		if record == "" {
			continue
		}
		args := strings.Split(record, "\x00")
		calls = append(calls, args[:len(args)-1])
	}

	return calls
}

// Sleeping reports whether the subcommand has installed its interrupt handler
// and started sleeping, so an interrupt sent now is handled cleanly.
func (s *stubTerraform) Sleeping(sub string) bool {
	_, err := os.Stat(filepath.Join(s.dir, sub+".sleeping"))
	return err == nil
}

// ClearSleeping forgets that the subcommand slept, so Sleeping reports the
// next invocation.
func (s *stubTerraform) ClearSleeping(sub string) {
	s.t.Helper()

	if err := os.Remove(filepath.Join(s.dir, sub+".sleeping")); err != nil {
		s.t.Fatal(err)
	}
}

func (s *stubTerraform) write(name, content string) {
	s.t.Helper()

	if err := os.WriteFile(filepath.Join(s.dir, name), []byte(content), 0o644); err != nil {
		s.t.Fatal(err)
	}
}

const nonEmptyPlanJSON = `{
  "format_version": "1.2",
  "terraform_version": "1.12.5",
  "planned_values": {
    "root_module": {
      "resources": [
        {
          "address": "terraform_data.a",
          "mode": "managed",
          "type": "terraform_data",
          "name": "a",
          "provider_name": "terraform.io/builtin/terraform",
          "schema_version": 0,
          "values": {}
        }
      ]
    }
  },
  "resource_changes": [
    {
      "address": "terraform_data.a",
      "mode": "managed",
      "type": "terraform_data",
      "name": "a",
      "provider_name": "terraform.io/builtin/terraform",
      "change": {"actions": ["create"], "before": null, "after": {}}
    }
  ]
}
`

func TestStubTerraform_CallsAreLossless(t *testing.T) {
	g := NewWithT(t)
	stub := newStubTerraform(t)

	want := []string{"init", "-backend-config=path=/a b/state", "-n", `x\ny`, "l1\nl2"}
	g.Expect(exec.Command(stub.Path(), want...).Run()).To(Succeed())

	g.Expect(stub.Calls()).To(Equal([][]string{want}))
}

func TestStubTerraform_Plan(t *testing.T) {
	newPlanSession := func(t *testing.T, h *harness, stub *stubTerraform) string {
		tf := &infrav1.Terraform{
			ObjectMeta: metav1.ObjectMeta{Name: "stub", Namespace: "default"},
		}
		return h.newSession(t, tf, stub.Path(), t.TempDir())
	}

	t.Run("state lock held", func(t *testing.T) {
		g := NewWithT(t)
		h := newHarness(t)
		stub := newStubTerraform(t)
		stub.On("plan", "", lockHeldStderr, 1)
		id := newPlanSession(t, h, stub)

		_, err := h.Client.Plan(t.Context(), &runner.PlanRequest{
			TfInstance: id,
			Out:        filepath.Join(h.Dir, "tfplan"),
			Refresh:    true,
		})

		g.Expect(status.Code(err)).To(Equal(codes.Aborted))
		detail, ok := stateLockDetail(err)
		g.Expect(ok).To(BeTrue(), "the StateLock detail is attached")
		g.Expect(detail.GetId()).To(Equal("9db590f1-b6fe-c5f2-2678-8804f089deba"))
		var subs []string
		for _, call := range stub.Calls() {
			subs = append(subs, call[0])
		}
		g.Expect(subs).To(ContainElement("plan"))
	})

	t.Run("drifted plan", func(t *testing.T) {
		g := NewWithT(t)
		h := newHarness(t)
		stub := newStubTerraform(t)
		stub.On("plan", "", "", 2)
		stub.On("show", nonEmptyPlanJSON, "", 0)
		id := newPlanSession(t, h, stub)
		out := filepath.Join(h.Dir, "tfplan")

		reply, err := h.Client.Plan(t.Context(), &runner.PlanRequest{
			TfInstance: id,
			Out:        out,
			Refresh:    true,
		})

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(reply.Drifted).To(BeTrue())
		g.Expect(reply.PlanCreated).To(BeTrue())
		g.Expect(out).To(BeAnExistingFile())
	})
}

// lockHeldStderr is real tofu 1.12.5 stderr for a plan that finds the state locked by another operation.
const lockHeldStderr = `
Error: Error acquiring the state lock

Error message: resource temporarily unavailable
Lock Info:
  ID:        9db590f1-b6fe-c5f2-2678-8804f089deba
  Path:      terraform.tfstate
  Operation: OperationTypePlan
  Who:       runner@tf-runner
  Version:   1.12.5
  Created:   2026-10-25 22:08:59.378353 +0000 UTC
  Info:      


OpenTofu acquires a state lock to protect the state from being written
by multiple users at the same time. Please resolve the issue above and try
again. For most commands, you can disable locking with the "-lock=false"
flag, but this is not recommended.
`

// lockAcquireFailedStderr is stderr when the state lock cannot be acquired and there is no lock holder to report.
const lockAcquireFailedStderr = `
Error: Error acquiring the state lock

Error message: open terraform.tfstate: permission denied

OpenTofu acquires a state lock to protect the state from being written
by multiple users at the same time. Please resolve the issue above and try
again. For most commands, you can disable locking with the "-lock=false"
flag, but this is not recommended.
`

// notLockedStderr is stderr of force-unlock against state that is not locked.
const notLockedStderr = `
Error: Failed to unlock state: LocalState not locked


`

// alreadyUnlockedStderr is stderr of force-unlock against the kubernetes
// backend when the state is not locked.
const alreadyUnlockedStderr = `
Error: Failed to unlock state: state is already unlocked


`

// lockMismatchStderr is stderr of force-unlock against the kubernetes backend
// when the state is locked by a holder other than the one named.
const lockMismatchStderr = `
Error: Failed to unlock state: lock id "abc" does not match existing lock
Lock Info:
  ID:        5b4e9f2a-1c3d-4e5f-8a9b-0c1d2e3f4a5b
  Path:      default/tfstate-default-helloworld
  Operation: OperationTypeApply
  Who:       runner@tf-runner
  Version:   1.12.5
  Created:   2026-10-25 22:08:59.378353 +0000 UTC
  Info:      


`

// validationFailedStderr is stderr of a plan that fails variable validation and echoes the variable value.
const validationFailedStderr = `
Error: Invalid value for variable

  on main.tf line 1:
   1: variable "s" {
    ├────────────────
    │ var.s is "SUPERSECRET"

too long

This was checked by the validation rule at main.tf:3,2-12.
`
