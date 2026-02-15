package server

import (
	"context"
	"fmt"
	"log"
	"tungo/application/network/connection"
	"tungo/application/network/routing"
	"tungo/infrastructure/settings"

	"golang.org/x/sync/errgroup"
)

type Runner struct {
	deps          AppDependencies
	workerFactory connection.ServerWorkerFactory
	routerFactory connection.ServerTrafficRouterFactory
}

func NewRunner(
	deps AppDependencies,
	workerFactory connection.ServerWorkerFactory,
	routerFactory connection.ServerTrafficRouterFactory,
) *Runner {
	return &Runner{
		deps:          deps,
		workerFactory: workerFactory,
		routerFactory: routerFactory,
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

	return r.runWorkers(ctx)
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
	errGroup, errGroupCtx := errgroup.WithContext(ctx)
	s := r.workerSettings()
	routers := make([]routing.Router, 0, len(s))
	for _, setting := range s {
		router, err := r.createRouter(errGroupCtx, setting)
		if err != nil {
			return fmt.Errorf("could not create %s router: %w", setting.Protocol, err)
		}
		routers = append(routers, router)
	}
	for i, router := range routers {
		errGroup.Go(func() error {
			if routeErr := router.RouteTraffic(errGroupCtx); routeErr != nil {
				return fmt.Errorf("%s worker failed: %w", s[i].Protocol, routeErr)
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
		return nil, fmt.Errorf("error creating worker: %w", workerErr)
	}
	return r.routerFactory.CreateRouter(worker), nil
}
