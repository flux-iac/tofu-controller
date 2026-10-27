package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/go-logr/logr"
	"github.com/hashicorp/terraform-exec/tfexec"
	tfjson "github.com/hashicorp/terraform-json"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	stateLockInfoRegexp = regexp.MustCompile(`Lock Info:\n\s*ID:\s*([^\n]+)\n\s*Path:\s*([^\n]+)\n\s*Operation:\s*([^\n]+)\n\s*Who:\s*([^\n]+)\n\s*Version:\s*([^\n]+)\n\s*Created:\s*([^\n]+)(?:\n|$)`)
	notLockedRegexp     = regexp.MustCompile(`(?i)not locked|already unlocked|no lock info found|blob metadata "?terraformlockid"? was empty|storage: object doesn't exist|lock id .* does not match`)
	varValueRegexp      = regexp.MustCompile(`(?m)^(\s*[│|]?\s*var\.[A-Za-z0-9_-]+(?:\.[A-Za-z0-9_-]+|\[[^\]]*\])* is )(.*)$`)
)

// minWaitDelay is the shortest time terraform gets to exit after it is
// interrupted before it is killed.
const minWaitDelay = 60 * time.Second

// StateLockErr is returned when there is an active State Lock
type StateLockError struct {
	originalError error

	ID        string
	Path      string
	Operation string
	Who       string
	Version   string
	Created   string
}

func (e *StateLockError) Error() string {
	if e.ID == "" {
		return fmt.Sprintf("error acquiring the state lock: %s", e.originalError.Error())
	}

	tmpl := `Lock Info:
  ID:        {{.ID}}
  Path:      {{.Path}}
  Operation: {{.Operation}}
  Who:       {{.Who}}
  Version:   {{.Version}}
  Created:   {{.Created}}
`

	t := template.Must(template.New("LockInfo").Parse(tmpl))
	var out strings.Builder
	if err := t.Execute(&out, e); err != nil {
		return "error acquiring the state lock"
	}
	return fmt.Sprintf("error acquiring the state lock: %v", out.String())
}

// Unwrap returns the error terraform-exec returned.
func (e *StateLockError) Unwrap() error { return e.originalError }

// sanitizedError is a terraform error whose message has had variable values
// masked. It still unwraps to the error terraform-exec returned.
type sanitizedError struct {
	msg string
	err error
}

func (e *sanitizedError) Error() string { return e.msg }

func (e *sanitizedError) Unwrap() error { return e.err }

// TerraformExecWrapper runs terraform commands. Every command's error goes
// through normalize, which classifies state locks and interruptions and
// masks variable values, and every command sets its own output writers.
//
// The writers are set on the shared tfexec.Terraform immediately before each
// command. That is safe because the controller calls the runner strictly in
// sequence, so no two commands run at the same time.
type TerraformExecWrapper struct {
	tf *tfexec.Terraform

	// quiet is set by DISABLE_TF_LOGS=1 and keeps terraform's output out of
	// the runner's stdout and stderr.
	quiet bool
	// sensitive is set by ENABLE_SENSITIVE_TF_LOGS=1 and logs every
	// terraform invocation, including its environment.
	sensitive bool

	waitDelay time.Duration
}

// NewTerraformExecWrapper wraps tf. Terraform gets grace, but never less than
// a minute, to exit after it is interrupted.
func NewTerraformExecWrapper(tf *tfexec.Terraform, grace time.Duration) *TerraformExecWrapper {
	t := &TerraformExecWrapper{
		tf:        tf,
		quiet:     os.Getenv("DISABLE_TF_LOGS") == "1",
		sensitive: os.Getenv("ENABLE_SENSITIVE_TF_LOGS") == "1",
		waitDelay: max(minWaitDelay, grace),
	}

	// SetWaitDelay only fails on Windows, where the runner does not run.
	_ = tf.SetWaitDelay(t.waitDelay)

	if !t.quiet && t.sensitive {
		tf.SetLogger(&LocalPrintfer{logger: ctrl.Log.WithName("runner").WithName("terraform")})
	}

	return t
}

// streamOutput sends terraform's output to the runner's stdout and stderr as
// it is written, unless terraform output is disabled. Both are masked line by
// line, because diagnostics echo variable values, and warnings such as failed
// check assertions are diagnostics printed to stdout. The returned function
// writes out a last line that has no newline once the command has finished.
func (t *TerraformExecWrapper) streamOutput() func() {
	if t.quiet {
		t.discardOutput()
		return func() {}
	}

	stdout := &sanitizingWriter{w: os.Stdout}
	stderr := &sanitizingWriter{w: os.Stderr}
	t.tf.SetStdout(stdout)
	t.tf.SetStderr(stderr)

	return func() {
		stdout.flush()
		stderr.flush()
	}
}

// sanitizingWriter writes each complete line through sanitize. It keeps the
// previous line, because sanitize masks the line after a
// generated.auto.tfvars.json reference.
type sanitizingWriter struct {
	w       io.Writer
	pending []byte
	prev    string
}

func (s *sanitizingWriter) Write(p []byte) (int, error) {
	s.pending = append(s.pending, p...)
	for {
		i := bytes.IndexByte(s.pending, '\n')
		if i < 0 {
			return len(p), nil
		}
		line := string(s.pending[:i])
		s.pending = s.pending[i+1:]
		if err := s.writeLine(line, "\n"); err != nil {
			return len(p), err
		}
	}
}

func (s *sanitizingWriter) writeLine(line, end string) error {
	_, masked, _ := strings.Cut(sanitize(s.prev+"\n"+line), "\n")
	s.prev = line
	_, err := io.WriteString(s.w, masked+end)

	return err
}

func (s *sanitizingWriter) flush() {
	if len(s.pending) == 0 {
		return
	}
	line := string(s.pending)
	s.pending = nil
	_ = s.writeLine(line, "")
}

// discardOutput keeps a command's output out of the runner's logs.
func (t *TerraformExecWrapper) discardOutput() {
	t.tf.SetStdout(io.Discard)
	t.tf.SetStderr(io.Discard)
}

// collectStderr discards stdout and collects stderr into a buffer, so
// printFailure can print it once the command has finished.
func (t *TerraformExecWrapper) collectStderr() *bytes.Buffer {
	var buf bytes.Buffer
	t.tf.SetStdout(io.Discard)
	t.tf.SetStderr(&buf)

	return &buf
}

// printFailure prints the collected stderr, with variable values masked, when
// the command failed for a reason other than a held state lock. A held lock
// is reported to the controller, and a successful command has nothing worth
// printing.
func (t *TerraformExecWrapper) printFailure(stderr *bytes.Buffer, err error) {
	var lockErr *StateLockError
	if t.quiet || err == nil || errors.As(err, &lockErr) || stderr.Len() == 0 {
		return
	}

	fmt.Fprint(os.Stderr, sanitize(stderr.String()))
}

func (t *TerraformExecWrapper) Init(ctx context.Context, opts ...tfexec.InitOption) error {
	stderr := t.collectStderr()

	err := t.normalize(ctx, t.tf.Init(ctx, opts...))
	t.printFailure(stderr, err)

	return err
}

func (t *TerraformExecWrapper) WorkspaceNew(ctx context.Context, workspace string, opts ...tfexec.WorkspaceNewCmdOption) error {
	defer t.streamOutput()()

	return t.normalize(ctx, t.tf.WorkspaceNew(ctx, workspace, opts...))
}

func (t *TerraformExecWrapper) WorkspaceSelect(ctx context.Context, workspace string, opts ...tfexec.WorkspaceSelectOption) error {
	defer t.streamOutput()()

	return t.normalize(ctx, t.tf.WorkspaceSelect(ctx, workspace, opts...))
}

func (t *TerraformExecWrapper) Plan(ctx context.Context, opts ...tfexec.PlanOption) (bool, error) {
	stderr := t.collectStderr()

	drifted, err := t.tf.Plan(ctx, opts...)
	err = t.normalize(ctx, err)
	t.printFailure(stderr, err)

	return drifted, err
}

func (t *TerraformExecWrapper) ShowPlanFile(ctx context.Context, planPath string, opts ...tfexec.ShowOption) (*tfjson.Plan, error) {
	t.discardOutput()

	plan, err := t.tf.ShowPlanFile(ctx, planPath, opts...)
	return plan, t.normalize(ctx, err)
}

func (t *TerraformExecWrapper) ShowPlanFileRaw(ctx context.Context, planPath string, opts ...tfexec.ShowOption) (string, error) {
	t.discardOutput()

	raw, err := t.tf.ShowPlanFileRaw(ctx, planPath, opts...)
	return raw, t.normalize(ctx, err)
}

func (t *TerraformExecWrapper) Apply(ctx context.Context, opts ...tfexec.ApplyOption) error {
	defer t.streamOutput()()

	return t.normalize(ctx, t.tf.Apply(ctx, opts...))
}

func (t *TerraformExecWrapper) Destroy(ctx context.Context, opts ...tfexec.DestroyOption) error {
	defer t.streamOutput()()

	return t.normalize(ctx, t.tf.Destroy(ctx, opts...))
}

func (t *TerraformExecWrapper) Output(ctx context.Context, opts ...tfexec.OutputOption) (map[string]tfexec.OutputMeta, error) {
	t.discardOutput()

	outputs, err := t.tf.Output(ctx, opts...)
	return outputs, t.normalize(ctx, err)
}

func (t *TerraformExecWrapper) Show(ctx context.Context, opts ...tfexec.ShowOption) (*tfjson.State, error) {
	t.discardOutput()

	state, err := t.tf.Show(ctx, opts...)
	return state, t.normalize(ctx, err)
}

func (t *TerraformExecWrapper) ForceUnlock(ctx context.Context, lockID string, opts ...tfexec.ForceUnlockOption) error {
	defer t.streamOutput()()

	return t.normalizeForceUnlock(ctx, lockID, t.tf.ForceUnlock(ctx, lockID, opts...))
}

func (t *TerraformExecWrapper) SetEnv(env map[string]string) error {
	return t.tf.SetEnv(env)
}

func (t *TerraformExecWrapper) WorkingDir() string {
	return t.tf.WorkingDir()
}

// normalize turns the error of a terraform command into the error the runner
// reports: an interruption, a held state lock, or the original error with
// variable values masked.
func (t *TerraformExecWrapper) normalize(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	if ctx.Err() != nil {
		reason := "request cancelled"
		switch cause := context.Cause(ctx); {
		case errors.Is(cause, errShuttingDown):
			reason = "runner shutting down"
		case errors.Is(cause, context.DeadlineExceeded):
			reason = "deadline exceeded"
		}

		return &InterruptedError{Reason: reason, Err: sanitized(err)}
	}

	// Without a Lock Info block there is no lock holder to report or
	// unlock, so a failure to acquire the lock is an ordinary error.
	if matches := stateLockInfoRegexp.FindStringSubmatch(strings.TrimSpace(err.Error())); matches != nil {
		return &StateLockError{
			originalError: err,
			ID:            matches[1],
			Path:          matches[2],
			Operation:     matches[3],
			Who:           matches[4],
			Version:       matches[5],
			Created:       matches[6],
		}
	}

	return sanitized(err)
}

// normalizeForceUnlock is normalize for force-unlock, which also reports that
// the lock it was asked to remove is not held.
func (t *TerraformExecWrapper) normalizeForceUnlock(ctx context.Context, lockID string, err error) error {
	err = t.normalize(ctx, err)

	// An interrupted force-unlock says nothing about whether the lock is
	// held, whatever terraform printed before it stopped.
	var interruptedErr *InterruptedError
	if errors.As(err, &interruptedErr) {
		return err
	}

	var sanitizedErr *sanitizedError
	if errors.As(err, &sanitizedErr) && notLockedRegexp.MatchString(sanitizedErr.msg) {
		return &NotLockedError{ID: lockID, Err: err}
	}

	return err
}

func sanitized(err error) error {
	return &sanitizedError{msg: sanitize(err.Error()), err: err}
}

// sanitize masks variable values that terraform echoes in its diagnostics:
// the JSON of generated.auto.tfvars.json and "var.name is <value>" lines.
func sanitize(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		if strings.Contains(lines[i], "on generated.auto.tfvars.json line") {
			if i+1 < len(lines) {
				// Extract the JSON part after the line number
				parts := strings.SplitN(lines[i+1], ": ", 2)
				if len(parts) < 2 {
					continue
				}
				var jsonObj map[string]any
				if err := json.Unmarshal([]byte(parts[1]), &jsonObj); err != nil {
					continue
				}
				for key := range jsonObj {
					jsonObj[key] = "***"
				}
				sanitizedJson, err := json.Marshal(jsonObj)
				if err != nil {
					continue
				}
				lines[i+1] = parts[0] + ": " + string(sanitizedJson)
			}
		}
	}

	return varValueRegexp.ReplaceAllString(strings.Join(lines, "\n"), "${1}***")
}

type LocalPrintfer struct {
	logger logr.Logger
}

func (l LocalPrintfer) Printf(format string, v ...any) {
	l.logger.Info(fmt.Sprintf(format, v...))
}
