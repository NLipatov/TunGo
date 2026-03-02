package client

import (
	"context"
	"errors"
	"fmt"
	"log"
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
			log.Printf("error disposing tun devices on exit: %s", err)
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
			log.Printf("session error: %v, reconnecting…", err)
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
		log.Printf("error disposing tun devices: %v", err)
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

	log.Printf("tunneling traffic via tun device")
	return router.RouteTraffic(ctx)
}

// runSessionInteractive handles the TUI path: dashboard starts before connection.
func (r *Runner) runSessionInteractive(ctx context.Context) error {
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	readyCh := make(chan struct{})
	uiOptions := runtimeUI.RuntimeUIOptions{
		ReadyCh: readyCh,
		Address: runnerCommon.RuntimeAddressInfoFromClientConfiguration(r.deps.Configuration()),
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
		log.Printf("tunneling traffic via tun device")

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
		func(err error) { log.Printf("runtime UI error: %v", err) },
	)
}
