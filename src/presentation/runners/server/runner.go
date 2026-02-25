package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"tungo/application/network/connection"
	"tungo/domain/app"
	"tungo/application/network/routing"
	"tungo/infrastructure/settings"
	runnerCommon "tungo/presentation/runners/common"
	runtimeUI "tungo/presentation/ui/tui"

	"golang.org/x/sync/errgroup"
)

type RuntimeDashboardFunc func(ctx context.Context, mode runtimeUI.RuntimeMode) (bool, error)

type Runner struct {
	uiMode              app.UIMode
	deps                AppDependencies
	workerFactory       connection.ServerWorkerFactory
	routerFactory       connection.ServerTrafficRouterFactory
	runRuntimeDashboard RuntimeDashboardFunc
}

type runtimeUIResult struct {
	userQuit bool
	err      error
}

func NewRunner(
	uiMode app.UIMode,
	deps AppDependencies,
	workerFactory connection.ServerWorkerFactory,
	routerFactory connection.ServerTrafficRouterFactory,
) *Runner {
	return &Runner{
		uiMode:              uiMode,
		deps:                deps,
		workerFactory:       workerFactory,
		routerFactory:       routerFactory,
		runRuntimeDashboard: runtimeUI.RunRuntimeDashboard,
	}
}

func (r *Runner) Run(
	ctx context.Context,
) error {
	err := r.deps.KeyManager().PrepareKeys()
	if err != nil {
		return fmt.Errorf("failed to generate ed25519 keys: %w", err)
	}

	// Pre-flight cleanup (if anything to clean up)
	if preflightCleanupErr := r.cleanup(); preflightCleanupErr != nil {
		log.Printf("preflight cleanup error: %v", preflightCleanupErr)
	}
	// Post-flight cleanup
	defer func() {
		if postflightCleanupErr := r.cleanup(); postflightCleanupErr != nil {
			log.Printf("postflight cleanup error: %v", postflightCleanupErr)
		}
	}()

	if r.uiMode != app.TUI {
		return r.runWorkers(ctx)
	}

	workersCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	workerErrCh := make(chan error, 1)
	go func() {
		workerErrCh <- r.runWorkers(workersCtx)
	}()

	uiResultCh := make(chan runtimeUIResult, 1)
	go func() {
		userQuit, err := r.runRuntimeDashboard(workersCtx, runtimeUI.RuntimeModeServer)
		uiResultCh <- runtimeUIResult{userQuit: userQuit, err: err}
	}()

	for {
		select {
		case workerErr := <-workerErrCh:
			cancel()
			uiResult := <-uiResultCh
			if uiResult.err != nil && !errors.Is(uiResult.err, runtimeUI.ErrUserExit) {
				log.Printf("runtime UI error: %v", uiResult.err)
			}
			return workerErr
		case uiResult := <-uiResultCh:
			if uiResult.err != nil {
				if errors.Is(uiResult.err, runtimeUI.ErrUserExit) {
					cancel()
					workerErr := <-workerErrCh
					if workerErr == nil || errors.Is(workerErr, context.Canceled) {
						return context.Canceled
					}
					return workerErr
				}
				cancel()
				workerErr := <-workerErrCh
				if workerErr == nil || errors.Is(workerErr, context.Canceled) {
					return fmt.Errorf("runtime UI failed: %w", uiResult.err)
				}
				return workerErr
			}
			if uiResult.userQuit {
				cancel()
				workerErr := <-workerErrCh
				if workerErr == nil || errors.Is(workerErr, context.Canceled) {
					return runnerCommon.ErrReconfigureRequested
				}
				return workerErr
			}
			cancel()
			return <-workerErrCh
		}
	}
}

func (r *Runner) cleanup() error {
	var eg errgroup.Group
	for _, ws := range r.deps.Configuration().AllSettings() {
		eg.Go(func() error {
			return r.deps.TunManager().DisposeDevices(ws)
		})
	}
	return eg.Wait()
}

func (r *Runner) runWorkers(
	ctx context.Context,
) error {
	workersCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errGroup, errGroupCtx := errgroup.WithContext(workersCtx)
	s := r.workerSettings()
	for _, setting := range s {
		router, err := r.createRouter(errGroupCtx, setting)
		if err != nil {
			// Ensure already started workers are asked to stop before return.
			cancel()
			_ = errGroup.Wait()
			return fmt.Errorf("could not create %s router: %w", setting.Protocol, err)
		}
		proto := setting.Protocol
		errGroup.Go(func() error {
			if routeErr := router.RouteTraffic(errGroupCtx); routeErr != nil {
				return fmt.Errorf("%s worker failed: %w", proto, routeErr)
			}
			return nil
		})
	}
	return errGroup.Wait()
}

func (r *Runner) workerSettings() []settings.Settings {
	return r.deps.Configuration().EnabledSettings()
}

func (r *Runner) createRouter(
	ctx context.Context,
	settings settings.Settings,
) (routing.Router, error) {
	tun, tunErr := r.deps.TunManager().CreateDevice(settings)
	if tunErr != nil {
		return nil, fmt.Errorf("error creating tun device: %w", tunErr)
	}
	worker, workerErr := r.workerFactory.CreateWorker(ctx, tun, settings)
	if workerErr != nil {
		_ = tun.Close()
		return nil, fmt.Errorf("error creating worker: %w", workerErr)
	}
	return r.routerFactory.CreateRouter(worker), nil
}
