package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"sigs.k8s.io/controller-runtime"
)

func (r *TerraformRunnerServer) ShowPlanFileRaw(ctx context.Context, req *ShowPlanFileRawRequest) (*ShowPlanFileRawReply, error) {
	log := controllerruntime.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("show the raw plan file")
	if req.TfInstance != r.InstanceID {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "no terraform")
		return nil, err
	}

	rawOutput, err := r.tf.ShowPlanFileRaw(ctx, req.Filename)
	if err != nil {
		log.Error(err, "unable to get the raw plan output")
		return nil, err
	}

	return &ShowPlanFileRawReply{RawOutput: rawOutput}, nil
}

func (r *TerraformRunnerServer) ShowPlanFile(ctx context.Context, req *ShowPlanFileRequest) (*ShowPlanFileReply, error) {
	log := controllerruntime.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("show the raw plan file")
	if req.TfInstance != r.InstanceID {
		err := fmt.Errorf("no TF instance found")
		log.Error(err, "no terraform")
		return nil, err
	}

	plan, err := r.tf.ShowPlanFile(ctx, req.Filename)
	if err != nil {
		log.Error(err, "unable to get the json plan output")
		return nil, err
	}

	jsonBytes, err := json.Marshal(plan)
	if err != nil {
		log.Error(err, "unable to marshal the plan to json")
		return nil, err
	}

	return &ShowPlanFileReply{JsonOutput: jsonBytes}, nil
}
