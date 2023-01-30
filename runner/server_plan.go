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

	return &PlanReply{Message: "ok", Drifted: drifted}, nil
}
