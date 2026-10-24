package controllers

import (
	"context"
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *TerraformReconciler) ShutdownCoordinator(ctx context.Context) {
	// Set shutdown flag to prevent new reconciliations from starting
	r.shutdownStarted.Store(true)

	if r.ShutdownTimeout.Seconds() < 1 {
		return
	}

	log := ctrl.LoggerFrom(ctx).WithName("shutdown-coordinator")

	shutdownStart := time.Now()

	log.Info("Graceful shutdown started",
		"timeout", r.ShutdownTimeout.String())

	done := make(chan struct{})
	go func() {
		r.activeReconciliations.Wait()
		close(done)
	}()

	timeout := time.After(r.ShutdownTimeout)

	select {
	case <-done:
		log.Info("All active reconciliations completed successfully",
			"totalWaitTime", time.Since(shutdownStart).String())
	case <-timeout:
		log.Error(fmt.Errorf("graceful shutdown timeout exceeded"),
			"Forcing shutdown with active reconciliations still running",
			"timeout", r.ShutdownTimeout.String(),
			"timeElapsed", time.Since(shutdownStart).String())
	}
}
