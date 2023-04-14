package runner

import (
	"context"
	"errors"
	"fmt"
	"github.com/hashicorp/terraform-exec/tfexec"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"sigs.k8s.io/controller-runtime"
)

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

	for _, target := range req.Targets {
		planOpt = append(planOpt, tfexec.Target(target))
	}

	drifted, err := r.tf.Plan(ctx, planOpt...)
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
		plan, err := r.tf.ShowPlanFile(ctx, req.Out)
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
