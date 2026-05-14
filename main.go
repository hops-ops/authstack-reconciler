// authstack-reconciler is a small in-cluster reconciler for the
// AuthStack XRD. It mints and validates the operational PATs
// (iam-admin-pat, login-client) against an existing Zitadel install,
// reading the durable machine-key Secret as its own credential to call
// Zitadel's API.
//
// See hops-ops/auth-stack:specs/authstack-reconciler for the design.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hops-ops/authstack-reconciler/internal/config"
	"github.com/hops-ops/authstack-reconciler/internal/reconciler"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.FromEnv()
	if err != nil {
		logger.Error("invalid config", "err", err)
		os.Exit(2)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Hard timeout so a stuck Zitadel doesn't pin the Job forever.
	ctx, cancelTimeout := context.WithTimeout(ctx, 10*time.Minute)
	defer cancelTimeout()

	r, err := reconciler.New(ctx, cfg, logger)
	if err != nil {
		logger.Error("failed to initialize reconciler", "err", err)
		os.Exit(1)
	}

	if err := r.Run(ctx); err != nil {
		logger.Error("reconcile failed", "err", err)
		os.Exit(1)
	}

	logger.Info("reconcile complete")
	_ = fmt.Sprintf // keep import for future error wrapping
}
