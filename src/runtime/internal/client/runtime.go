package client

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	palClient "tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/logging"
	"tungo/infrastructure/tunnel/sessionplane/client_factory"
)

type Runtime struct {
	runner *Runner
}

func NewRuntime() (*Runtime, error) {
	setupCrashLog()
	slog.Info("starting client")

	deps := NewDependencies(palClient.NewManager())
	if err := deps.Initialize(); err != nil {
		return nil, fmt.Errorf("init error: %w", err)
	}

	routerFactory := client_factory.NewRouterFactory()
	return &Runtime{
		runner: NewRunner(deps, routerFactory),
	}, nil
}

func (r *Runtime) Run(ctx context.Context) error {
	err := r.runner.Run(ctx)
	if err == nil || ctx.Err() != nil ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return err
}

func (r *Runtime) WaitForReady(ctx context.Context) error {
	return r.runner.WaitForReady(ctx)
}

func setupCrashLog() {
	configPath, err := palClient.NewDefaultResolver().Resolve()
	if err != nil {
		return
	}
	logging.SetCrashOutput(filepath.Join(filepath.Dir(configPath), "crash.log"))
}
