package runner

import (
	"context"
	"encoding/json"

	controllerruntime "sigs.k8s.io/controller-runtime"
)

func (r *TerraformRunnerServer) ShowPlanFileRaw(ctx context.Context, req *ShowPlanFileRawRequest) (*ShowPlanFileRawReply, error) {
	log := controllerruntime.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("show the raw plan file")

	if err := r.ValidateInstanceID(req.TfInstance); err != nil {
		log.Error(err, "terraform session mismatch when showing the raw plan file")

		return nil, err
	}

	rawOutput, err := r.tfShowPlanFileRaw(ctx, req.Filename)
	if err != nil {
		log.Error(err, "unable to get the raw plan output")
		return nil, err
	}

	return &ShowPlanFileRawReply{RawOutput: rawOutput}, nil
}

func (r *TerraformRunnerServer) ShowPlanFile(ctx context.Context, req *ShowPlanFileRequest) (*ShowPlanFileReply, error) {
	log := controllerruntime.LoggerFrom(ctx, "instance-id", r.InstanceID).WithName(loggerName)
	log.Info("show the plan file")

	if err := r.ValidateInstanceID(req.TfInstance); err != nil {
		log.Error(err, "terraform session mismatch when showing the plan file")

		return nil, err
	}

	plan, err := r.tfShowPlanFile(ctx, req.Filename)
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
