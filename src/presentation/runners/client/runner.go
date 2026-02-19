package client

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
	"tungo/application/network/connection"
	runnerCommon "tungo/presentation/runners/common"
	runtimeUI "tungo/presentation/ui/tui"
)

type Runner struct {
	deps          AppDependencies
	routerFactory connection.TrafficRouterFactory
}

var (
	isInteractiveRuntime = runtimeUI.IsInteractiveRuntime
	runRuntimeDashboard  = runtimeUI.RunRuntimeDashboard
)

type runtimeUIResult struct {
	userQuit bool
	err      error
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

	router, conn, tun, err := r.routerFactory.
		CreateRouter(ctx, r.deps.ConnectionFactory(), r.deps.TunManager(), r.deps.WorkerFactory())
	if err != nil {
		return fmt.Errorf("failed to create router: %s", err)
	}

	go func() {
		<-ctx.Done() //blocks until context is cancelled
		_ = conn.Close()
		_ = tun.Close()
	}()

	log.Printf("tunneling traffic via tun device")
	if !isInteractiveRuntime() {
		return router.RouteTraffic(ctx)
	}
	routeErrCh := make(chan error, 1)
	go func() {
		routeErrCh <- router.RouteTraffic(ctx)
	}()

	uiResultCh := make(chan runtimeUIResult, 1)
	go func() {
		userQuit, err := runRuntimeDashboard(ctx, runtimeUI.RuntimeModeClient)
		uiResultCh <- runtimeUIResult{userQuit: userQuit, err: err}
	}()

	for {
		select {
		case routeErr := <-routeErrCh:
			cancel()
			uiResult := <-uiResultCh
			if uiResult.err != nil && !errors.Is(uiResult.err, runtimeUI.ErrUserExit) {
				log.Printf("runtime UI error: %v", uiResult.err)
			}
			return routeErr
		case uiResult := <-uiResultCh:
			if uiResult.err != nil {
				if errors.Is(uiResult.err, runtimeUI.ErrUserExit) {
					cancel()
					routeErr := <-routeErrCh
					if routeErr == nil || errors.Is(routeErr, context.Canceled) {
						return context.Canceled
					}
					return routeErr
				}
				cancel()
				routeErr := <-routeErrCh
				if routeErr == nil || errors.Is(routeErr, context.Canceled) {
					return fmt.Errorf("runtime UI failed: %w", uiResult.err)
				}
				return routeErr
			}
			if uiResult.userQuit {
				cancel()
				routeErr := <-routeErrCh
				if routeErr == nil || errors.Is(routeErr, context.Canceled) {
					return runnerCommon.ErrReconfigureRequested
				}
				return routeErr
			}
			return <-routeErrCh
		}
	}
}
