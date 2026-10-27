package controllers

import (
	"context"
	"fmt"
	"time"

	infrav1 "github.com/flux-iac/tofu-controller/api/v1alpha2"
	"github.com/flux-iac/tofu-controller/runner"
	ctrl "sigs.k8s.io/controller-runtime"
)

// breakGlassPollInterval is how often breakGlass checks whether the session has ended.
const breakGlassPollInterval = 10 * time.Second

// breakGlassRequested reports whether spec.breakTheGlass or the break-the-glass
// annotation asks for a break-the-glass session.
func breakGlassRequested(obj *infrav1.Terraform) bool {
	if obj.Spec.BreakTheGlass {
		return true
	}
	_, exist := obj.Annotations[infrav1.BreakTheGlassAnnotation]
	return exist
}

// breakGlass runs a break-the-glass session when allowed. It always
// returns an error: the session ending, or break the glass not being allowed.
func (r *TerraformReconciler) breakGlass(ctx context.Context, s *runnerSession, obj *infrav1.Terraform) error {
	log := ctrl.LoggerFrom(ctx)

	if !r.AllowBreakTheGlass {
		infrav1.TerraformProgressing(obj, "Breaking the glass is not allowed")
		if err := r.progress(ctx, s, obj, "unable to record progress"); err != nil {
			return err
		}

		log.Info("break the glass is not allowed")
		return fmt.Errorf("break the glass is not allowed")
	}

	infrav1.TerraformProgressing(obj, "Breaking the glass ...")
	if err := r.progress(ctx, s, obj, "unable to record progress"); err != nil {
		return err
	}

	// create /tmp/.break-glass file
	if _, err := s.client.StartBreakTheGlassSession(ctx, &runner.BreakTheGlassRequest{}); err != nil {
		log.Error(err, "error starting break the glass session")
		return err
	}

	// Poll until the session ends. The worker stays blocked meanwhile, because
	// tfctl shells into the runner pod, which the runner cleanup would
	// otherwise delete; a cancelled ctx (shutdown) ends the wait.
	ticker := time.NewTicker(breakGlassPollInterval)
	defer ticker.Stop()
	for done := false; !done; {
		// check /tmp/.break-glass file exists
		reply, err := s.client.HasBreakTheGlassSessionDone(ctx, &runner.BreakTheGlassRequest{})
		if err != nil {
			return err
		}
		done = reply.Success

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}

	// set status back to "Initializing"
	infrav1.TerraformProgressing(obj, "Initializing")
	if err := r.progress(ctx, s, obj, "unable to record progress"); err != nil {
		return err
	}

	return fmt.Errorf("break the glass session has ended at %v", time.Now().Format(time.RFC3339))
}
