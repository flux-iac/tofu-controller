package main

import (
	"context"
	"os"

	flag "github.com/spf13/pflag"
	"github.com/weaveworks/tf-controller/internal/server/webhook"

	"github.com/fluxcd/pkg/runtime/logger"
)

func main() {
	var (
		logOptions logger.Options
		serverAddr string
	)

	flag.StringVar(&serverAddr, "bind-address", webhook.DefaultListenAddress, "The address the webhook server endpoint binds to.")
	logOptions.BindFlags(flag.CommandLine)

	flag.Parse()

	log := logger.NewLogger(logOptions)
	server, err := webhook.New(
		webhook.WithLogger(log),
	)
	if err != nil {
		log.Error(err, "problem configuring the webhook receiver server")
		os.Exit(1)
	}

	ctx := context.Background()

	if err := server.Start(ctx); err != nil {
		log.Error(err, "problem running the webhook receiver server")
		os.Exit(1)
	}
}
