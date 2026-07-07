package client

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
	"tungo/application/network/connection"
)

type Runner struct {
	deps          AppDependencies
	routerFactory connection.TrafficRouterFactory
}

type RunOptions struct {
	ReadyCh chan<- struct{}
}

func NewRunner(deps AppDependencies, routerFactory connection.TrafficRouterFactory) *Runner {
	return &Runner{
		deps:          deps,
		routerFactory: routerFactory,
	}
}

func (r *Runner) Run(ctx context.Context, options RunOptions) error {
	defer func() {
		if err := r.deps.TunManager().DisposeDevices(); err != nil {
			slog.Warn("failed to dispose TUN devices on exit", "err", err)
		}
	}()

	var readyOnce sync.Once

	for ctx.Err() == nil {
		err := r.runAttempt(ctx, options.ReadyCh, &readyOnce)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, errReconfigureRequested):
			return err
		case errors.Is(err, context.Canceled):
			return context.Canceled
		default:
			slog.Warn("session error, reconnecting", "err", err)
			timer := time.NewTimer(500 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return context.Canceled
			case <-timer.C:
			}
		}
	}
	return context.Canceled
}

func (r *Runner) runAttempt(
	parentCtx context.Context,
	readyCh chan<- struct{},
	readyOnce *sync.Once,
) error {
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	if err := r.deps.TunManager().DisposeDevices(); err != nil {
		slog.Warn("failed to dispose TUN devices", "err", err)
	}

	router, conn, tun, err := r.routerFactory.
		CreateRouter(ctx, r.deps.ConnectionFactory(), r.deps.TunManager(), r.deps.WorkerFactory())
	if err != nil {
		return fmt.Errorf("failed to create router: %w", err)
	}
	if readyCh != nil && readyOnce != nil {
		readyOnce.Do(func() {
			close(readyCh)
		})
	}

	defer func() {
		_ = conn.Close()
		_ = tun.Close()
	}()

	slog.Info("tunneling traffic via TUN device")
	return router.RouteTraffic(ctx)
}
