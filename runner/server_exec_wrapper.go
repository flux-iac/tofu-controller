package runner

import (
	context "context"
	"fmt"
	"regexp"
	"strings"
	"text/template"

	"github.com/hashicorp/terraform-exec/tfexec"
)

var (
	stateLockErrRegexp  = regexp.MustCompile(`(?s)Error acquiring the state lock`)
	stateLockInfoRegexp = regexp.MustCompile(`Lock Info:\n\s*ID:\s*([^\n]+)\n\s*Path:\s*([^\n]+)\n\s*Operation:\s*([^\n]+)\n\s*Who:\s*([^\n]+)\n\s*Version:\s*([^\n]+)\n\s*Created:\s*([^\n]+)(?:\n|$)`)
)

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

// TerraformExecWrapper wraps tfexec.Terraform to normalise CLI errors, so we can detect State Locks
type TerraformExecWrapper struct {
	*tfexec.Terraform
}

func NewTerraformExecWrapper(tf *tfexec.Terraform) *TerraformExecWrapper {
	return &TerraformExecWrapper{Terraform: tf}
}

func (t *TerraformExecWrapper) Init(ctx context.Context, opts ...tfexec.InitOption) error {
	return t.NormalizeError(t.Terraform.Init(ctx, opts...))
}

func (t *TerraformExecWrapper) Apply(ctx context.Context, opts ...tfexec.ApplyOption) error {
	return t.NormalizeError(t.Terraform.Apply(ctx, opts...))
}

func (t *TerraformExecWrapper) Destroy(ctx context.Context, opts ...tfexec.DestroyOption) error {
	return t.NormalizeError(t.Terraform.Destroy(ctx, opts...))
}

func (t *TerraformExecWrapper) Plan(ctx context.Context, opts ...tfexec.PlanOption) (bool, error) {
	drifted, err := t.Terraform.Plan(ctx, opts...)
	return drifted, t.NormalizeError(err)
}

func (t *TerraformExecWrapper) NormalizeError(err error) error {
	if err == nil {
		return nil
	}

	trimmedError := strings.TrimSpace(err.Error())

	switch {
	case stateLockInfoRegexp.MatchString(trimmedError):
		matches := stateLockInfoRegexp.FindStringSubmatch(trimmedError)

		return &StateLockError{
			originalError: err,
			ID:            matches[1],
			Path:          matches[2],
			Operation:     matches[3],
			Who:           matches[4],
			Version:       matches[5],
			Created:       matches[6],
		}
	case stateLockErrRegexp.MatchString(trimmedError):
		return &StateLockError{originalError: err}
	}

	return err
}
