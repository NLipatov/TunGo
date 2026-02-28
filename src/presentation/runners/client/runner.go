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

type RuntimeDashboardFunc func(ctx context.Context, mode runtimeUI.RuntimeMode, readyCh <-chan struct{}) (bool, error)

type Runner struct {
	uiMode              app.UIMode
	deps                AppDependencies
	routerFactory       connection.TrafficRouterFactory
	runRuntimeDashboard RuntimeDashboardFunc
}

type runtimeUIResult struct {
	userQuit bool
	err      error
}

type connectResult struct {
	err error
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

	uiResultCh := make(chan runtimeUIResult, 1)
	go func() {
		userQuit, err := r.runRuntimeDashboard(sessionCtx, runtimeUI.RuntimeModeClient, readyCh)
		uiResultCh <- runtimeUIResult{userQuit: userQuit, err: err}
	}()

	connectCh := make(chan connectResult, 1)
	go func() {
		router, conn, tun, err := r.routerFactory.
			CreateRouter(sessionCtx, r.deps.ConnectionFactory(), r.deps.TunManager(), r.deps.WorkerFactory())
		if err != nil {
			connectCh <- connectResult{err: err}
			return
		}

		close(readyCh)
		log.Printf("tunneling traffic via tun device")

		go func() {
			<-sessionCtx.Done()
			_ = conn.Close()
			_ = tun.Close()
		}()

		connectCh <- connectResult{err: router.RouteTraffic(sessionCtx)}
	}()

	return r.waitForSessionEnd(cancel, uiResultCh, connectCh)
}

func (r *Runner) waitForSessionEnd(
	cancel context.CancelFunc,
	uiResultCh <-chan runtimeUIResult,
	connectCh <-chan connectResult,
) error {
	for {
		select {
		case cr := <-connectCh:
			// Connection/routing finished first — cancel UI, drain it, return route result.
			cancel()
			uiResult := <-uiResultCh
			if uiResult.err != nil && !errors.Is(uiResult.err, runtimeUI.ErrUserExit) {
				log.Printf("runtime UI error: %v", uiResult.err)
			}
			return cr.err
		case uiResult := <-uiResultCh:
			if uiResult.err != nil {
				// UI failed — cancel route, wait for route result.
				cancel()
				cr := <-connectCh
				if cr.err != nil && !errors.Is(cr.err, context.Canceled) {
					return cr.err
				}
				if errors.Is(uiResult.err, runtimeUI.ErrUserExit) {
					return context.Canceled
				}
				return fmt.Errorf("runtime UI failed: %w", uiResult.err)
			}
			if uiResult.userQuit {
				// Reconfigure requested — cancel route, wait for route result.
				cancel()
				cr := <-connectCh
				if cr.err != nil && !errors.Is(cr.err, context.Canceled) {
					return cr.err
				}
				return runnerCommon.ErrReconfigureRequested
			}
			// UI exited cleanly without quit — wait for route.
			cr := <-connectCh
			return cr.err
		}
	}
}
