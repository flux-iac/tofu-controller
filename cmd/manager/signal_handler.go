package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// This is the same as ctrl.SetupSignalHandler(), with the difference that it will run
// shutdown() callback before canceling the context. Second signal still exits immediately.
func setupSignalHandler(shutdownCaller *func()) context.Context {
	ctx, cancel := context.WithCancel(context.Background())

	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-c
		setupLog.Info("Received shutdown signal, initiating graceful shutdown", "signal", sig.String())

		// shutdown function gets updated only once, this part is safe for a goroutine
		if shutdown := *shutdownCaller; shutdown == nil {
			cancel()
		} else {
			go func(shutdown func(), cancel context.CancelFunc) {
				shutdown()
				cancel()
				setupLog.Info("Graceful shutdown completed")
			}(shutdown, cancel)
		}

		sig = <-c
		setupLog.Info("Received second signal, forcing immediate shutdown", "signal", sig.String())
		os.Exit(1)
	}()

	return ctx
}
