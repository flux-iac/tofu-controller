package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/hashicorp/terraform-exec/tfexec"
	tfjson "github.com/hashicorp/terraform-json"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	controllerruntime "sigs.k8s.io/controller-runtime"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *TerraformRunnerServer) tfShowPlanFile(ctx context.Context, planPath string, opts ...tfexec.ShowOption) (*tfjson.Plan, error) {
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)

	// This is the only place where we disable the logger
	r.tf.SetStdout(io.Discard)
	r.tf.SetStderr(io.Discard)

	defer r.initLogger(log)

	return r.tf.ShowPlanFile(ctx, planPath, opts...)
}

func (r *TerraformRunnerServer) tfShowPlanFileRaw(ctx context.Context, planPath string, opts ...tfexec.ShowOption) (string, error) {
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)

	// This is the only place where we disable the logger
	r.tf.SetStdout(io.Discard)
	r.tf.SetStderr(io.Discard)

	defer r.initLogger(log)

	return r.tf.ShowPlanFileRaw(ctx, planPath, opts...)
}

func sanitizeLog(log string) string {
	lines := strings.Split(log, "\n")
	for i := 0; i < len(lines); i++ {
		if strings.Contains(lines[i], "on generated.auto.tfvars.json line") {
			if i+1 < len(lines) {
				// Extract the JSON part after the line number
				parts := strings.SplitN(lines[i+1], ": ", 2)
				if len(parts) < 2 {
					continue
				}
				var jsonObj map[string]interface{}
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

	return strings.Join(lines, "\n")
}

func (r *TerraformRunnerServer) tfPlan(ctx context.Context, opts ...tfexec.PlanOption) (bool, error) {
	log := ctrl.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)

	// This is the only place where we disable the logger
	r.tf.SetStdout(io.Discard)
	errBuf := &bytes.Buffer{}
	r.tf.SetStderr(errBuf)

	defer r.initLogger(log)

	diff, err := r.tf.Plan(ctx, opts...)
	// sanitize the error message only if it's not a state lock error
	var sl *tfexec.ErrStateLocked
	if err != nil && errors.As(err, &sl) == false {
		fmt.Fprint(os.Stderr, sanitizeLog(errBuf.String()))
		err = errors.New(sanitizeLog(err.Error()))
	}

	return diff, err
}

func (r *TerraformRunnerServer) Plan(ctx context.Context, req *PlanRequest) (*PlanReply, error) {
	log := controllerruntime.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("creating a plan")
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-r.Done:
			cancel()
		case <-ctx.Done():
		}
	}()

	if req.TfInstance != r.InstanceID {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "no terraform")
		return nil, err
	}

	var planOpt []tfexec.PlanOption
	if req.Out != "" {
		planOpt = append(planOpt, tfexec.Out(req.Out))
	} else {
		// if backend is disabled completely, there will be no plan output file (req.Out = "")
		log.Info("backend seems to be disabled completely, so there will be no plan output file")
	}

	if req.Refresh == false {
		planOpt = append(planOpt, tfexec.Refresh(req.Refresh))
	}

	if req.Destroy {
		planOpt = append(planOpt, tfexec.Destroy(req.Destroy))
	}

	if req.DisablePlanLock {
		log.Info("Running plan with no lock")
		planOpt = append(planOpt, tfexec.Lock(false))
	}

	if req.LockTimeout != "" {
		planOpt = append(planOpt, tfexec.LockTimeout(req.LockTimeout))
	}

	for _, target := range req.Targets {
		planOpt = append(planOpt, tfexec.Target(target))
	}

	for _, path := range r.terraform.Spec.TfVarsFiles {
		secureTfVarsFile, err := securejoin.SecureJoin(req.SourceRefRootDir, path)
		if err != nil {
			log.Error(err, "Failed to secure join root dir with the given tfvars file's path.")
			return nil, err
		}

		info, err := os.Stat(secureTfVarsFile)
		if os.IsNotExist(err) {
			log.Error(err, "The given tfvars file's path does not exist.")
			return nil, fmt.Errorf("error running plan: tfvars file's path does not exist: %s", secureTfVarsFile)
		}

		if info.IsDir() {
			log.Error(err, "The given tfvars file's path does not exist.")
			return nil, fmt.Errorf("error running Plan: tfvars file's path is a directory: %s",
				secureTfVarsFile)
		}

		planOpt = append(planOpt, tfexec.VarFile(secureTfVarsFile))
	}

	drifted, err := r.tfPlan(ctx, planOpt...)
	if err != nil {
		st := status.New(codes.Internal, err.Error())
		var stateErr *tfexec.ErrStateLocked

		if errors.As(err, &stateErr) {
			st, err = st.WithDetails(&PlanReply{Message: "not ok", StateLockIdentifier: stateErr.ID})

			if err != nil {
				return nil, err
			}
		}

		log.Error(err, "error creating the plan")
		return nil, st.Err()
	}

	planCreated := false
	if req.Out != "" {
		planCreated = true

		plan, err := r.tfShowPlanFile(ctx, req.Out)
		if err != nil {
			return nil, err
		}

		// This is the case when the plan is empty.
		if plan.PlannedValues.Outputs == nil &&
			plan.PlannedValues.RootModule.Resources == nil &&
			plan.ResourceChanges == nil &&
			plan.PriorState == nil &&
			plan.OutputChanges == nil {
			planCreated = false
		}

	}

	return &PlanReply{Message: "ok", Drifted: drifted, PlanCreated: planCreated}, nil
}
