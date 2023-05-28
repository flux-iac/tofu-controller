package runner

import (
	"context"
	"os"

	controllerruntime "sigs.k8s.io/controller-runtime"
)

const BreakGlassFile = "/tmp/.break-glass"

func (r *TerraformRunnerServer) StartBreakTheGlassSession(ctx context.Context, req *BreakTheGlassRequest) (*BreakTheGlassReply, error) {
	log := controllerruntime.LoggerFrom(ctx).WithName(loggerName)
	log.Info("starting break the glass session")

	// create /tmp/.break-glass file
	err := os.WriteFile(BreakGlassFile, []byte("1"), 0644)
	if err != nil {
		return nil, err
	}

	return &BreakTheGlassReply{Message: "break the glass session started", Success: true}, nil
}

func (r *TerraformRunnerServer) HasBreakTheGlassSessionDone(ctx context.Context, req *BreakTheGlassRequest) (*BreakTheGlassReply, error) {
	log := controllerruntime.LoggerFrom(ctx).WithName(loggerName)
	log.Info("checking break the glass session")

	// check /tmp/.break-glass file exists
	_, err := os.Stat(BreakGlassFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &BreakTheGlassReply{Message: "break the glass session done", Success: true}, nil
		}
		return nil, err
	}

	return &BreakTheGlassReply{Message: "break the glass session not done", Success: false}, nil
}
