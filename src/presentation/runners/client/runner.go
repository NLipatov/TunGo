package client

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
	"tungo/application/network/connection"
	"tungo/domain/app"
	runnerCommon "tungo/presentation/runners/common"
	runtimeUI "tungo/presentation/ui/tui"
)

type RuntimeDashboardFunc func(ctx context.Context, mode runtimeUI.RuntimeMode, options runtimeUI.RuntimeUIOptions) (bool, error)

type Runner struct {
	uiMode              app.UIMode
	deps                AppDependencies
	routerFactory       connection.TrafficRouterFactory
	runRuntimeDashboard RuntimeDashboardFunc
}

func NewRunner(uiMode app.UIMode, deps AppDependencies, routerFactory connection.TrafficRouterFactory) *Runner {
	return &Runner{
		uiMode:              uiMode,
		deps:                deps,
		routerFactory:       routerFactory,
		runRuntimeDashboard: runtimeUI.RunRuntimeDashboard,
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
		case errors.Is(err, runnerCommon.ErrReconfigureRequested):
			return runnerCommon.ErrReconfigureRequested
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

func (r *Runner) runSession(parentCtx context.Context) error {
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	if err := r.deps.TunManager().DisposeDevices(); err != nil {
		slog.Warn("failed to dispose TUN devices", "err", err)
	}

	if r.uiMode != app.TUI {
		return r.runSessionBlocking(ctx)
	}
	return r.runSessionInteractive(ctx)
}

// runSessionBlocking handles the non-TUI (CLI) path: blocking connect then route.
func (r *Runner) runSessionBlocking(ctx context.Context) error {
	router, conn, tun, err := r.routerFactory.
		CreateRouter(ctx, r.deps.ConnectionFactory(), r.deps.TunManager(), r.deps.WorkerFactory())
	if err != nil {
		return fmt.Errorf("failed to create router: %s", err)
	}

	go func() {
		<-ctx.Done()
		_ = conn.Close()
		_ = tun.Close()
	}()

	slog.Info("tunneling traffic via TUN device")
	return router.RouteTraffic(ctx)
}

// runSessionInteractive handles the TUI path: dashboard starts before connection.
func (r *Runner) runSessionInteractive(ctx context.Context) error {
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	readyCh := make(chan struct{})
	uiOptions := runtimeUI.RuntimeUIOptions{
		ReadyCh:  readyCh,
		Address:  runnerCommon.RuntimeAddressInfoFromClientConfiguration(r.deps.Configuration()),
		Protocol: r.deps.Configuration().Protocol,
	}

	uiResultCh := make(chan runnerCommon.RuntimeUIResult, 1)
	go func() {
		userQuit, err := r.runRuntimeDashboard(sessionCtx, runtimeUI.RuntimeModeClient, uiOptions)
		uiResultCh <- runnerCommon.RuntimeUIResult{UserQuit: userQuit, Err: err}
	}()

	connectCh := make(chan error, 1)
	go func() {
		router, conn, tun, err := r.routerFactory.
			CreateRouter(sessionCtx, r.deps.ConnectionFactory(), r.deps.TunManager(), r.deps.WorkerFactory())
		if err != nil {
			connectCh <- err
			return
		}

		close(readyCh)
		slog.Info("tunneling traffic via TUN device")

		go func() {
			<-sessionCtx.Done()
			_ = conn.Close()
			_ = tun.Close()
		}()

		connectCh <- router.RouteTraffic(sessionCtx)
	}()

	return runnerCommon.WaitForRuntimeSessionEnd(
		cancel,
		uiResultCh,
		connectCh,
		func(err error) bool { return errors.Is(err, runtimeUI.ErrUserExit) },
		func(err error) { slog.Error("runtime UI error", "err", err) },
	)
}
