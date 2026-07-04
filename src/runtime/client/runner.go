package client

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
	"tungo/application/network/connection"
)

type SessionOptions struct {
	ReadyCh chan<- struct{}
}

type Runner struct {
	deps          AppDependencies
	routerFactory connection.TrafficRouterFactory
}

func NewRunner(deps AppDependencies, routerFactory connection.TrafficRouterFactory) *Runner {
	return &Runner{
		deps:          deps,
		routerFactory: routerFactory,
	}
}

func (r *Runner) Run(ctx context.Context) error {
	defer func() {
		if err := r.deps.TunManager().DisposeDevices(); err != nil {
			slog.Warn("failed to dispose TUN devices on exit", "err", err)
		}
	}()

	for ctx.Err() == nil {
		err := r.runSession(ctx)
		switch {
		case err == nil:
			return nil
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

func (r *Runner) RunSession(parentCtx context.Context, options SessionOptions) error {
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	if err := r.deps.TunManager().DisposeDevices(); err != nil {
		slog.Warn("failed to dispose TUN devices", "err", err)
	}

	router, conn, tun, err := r.routerFactory.
		CreateRouter(ctx, r.deps.ConnectionFactory(), r.deps.TunManager(), r.deps.WorkerFactory())
	if err != nil {
		return fmt.Errorf("failed to create router: %s", err)
	}
	if options.ReadyCh != nil {
		close(options.ReadyCh)
	}

	go func() {
		<-ctx.Done()
		_ = conn.Close()
		_ = tun.Close()
	}()

	slog.Info("tunneling traffic via TUN device")
	return router.RouteTraffic(ctx)
}
