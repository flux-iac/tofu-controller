package runner

import (
	"context"
	"errors"
	"maps"
	"os"
	"os/exec"
	"time"

	"github.com/hashicorp/terraform-exec/tfexec"
	ctrl "sigs.k8s.io/controller-runtime"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/utils"
)

// NewTerraform builds a new session and makes it the current one. When it
// fails, the previous session stays current.
func (r *TerraformRunnerServer) NewTerraform(ctx context.Context, req *NewTerraformRequest) (*NewTerraformReply, error) {
	log := ctrl.LoggerFrom(ctx).WithValues("instance-id", req.GetInstanceID())
	log.Info("creating new terraform", "workingDir", req.WorkingDir, "execPath", req.ExecPath)
	tf, err := tfexec.NewTerraform(req.WorkingDir, req.ExecPath)
	if err != nil {
		return nil, err
	}

	var terraform infrav1.Terraform
	if err := terraform.FromBytes(req.Terraform, r.Scheme); err != nil {
		return nil, err
	}

	var grace time.Duration
	if seconds := terraform.Spec.RunnerTerminationGracePeriodSeconds; seconds != nil {
		grace = time.Duration(*seconds) * time.Second
	}

	s := &session{
		id:        req.GetInstanceID(),
		tf:        NewTerraformExecWrapper(tf, grace),
		terraform: &terraform,
	}
	r.setSession(s)

	return &NewTerraformReply{Id: s.id}, nil
}

func (r *TerraformRunnerServer) SetEnv(ctx context.Context, req *SetEnvRequest) (*SetEnvReply, error) {
	ctrl.LoggerFrom(ctx).Info("setting envvars")

	envs := utils.EnvMap(os.Environ())
	maps.Copy(envs, req.Envs)
	if err := sessionFrom(ctx).tf.SetEnv(envs); err != nil {
		return nil, err
	}

	return &SetEnvReply{Message: "ok"}, nil
}

func (r *TerraformRunnerServer) LookPath(ctx context.Context, req *LookPathRequest) (*LookPathReply, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("looking for paths", "files", req.Files)

	for _, file := range req.Files {
		execPath, err := exec.LookPath(file)
		if err == nil {
			log.Info("found binary", "file", file, "execPath", execPath)
			return &LookPathReply{ExecPath: execPath}, nil
		}
	}

	return nil, errors.New("none of the specified binaries were found in the runners PATH")
}
