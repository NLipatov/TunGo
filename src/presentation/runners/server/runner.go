package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"tungo/application"
	"tungo/infrastructure/settings"

	"golang.org/x/sync/errgroup"
)

type Runner struct {
	deps          AppDependencies
	workerFactory application.ServerWorkerFactory
	routerFactory application.ServerTrafficRouterFactory
}

func NewRunner(
	deps AppDependencies,
	workerFactory application.ServerWorkerFactory,
	routerFactory application.ServerTrafficRouterFactory,
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
		return fmt.Errorf("failed to generate ed25519 keys: %s", err)
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
	for _, workerSettings := range r.workerSettings() {
		eg.Go(func() error {
			return r.deps.TunManager().DisposeTunDevices(workerSettings)
		})
	}

	return eg.Wait()
}

func (r *Runner) workerSettings() []settings.Settings {
	return []settings.Settings{
		r.deps.Configuration().TCPSettings,
		r.deps.Configuration().UDPSettings,
		r.deps.Configuration().WSSettings,
	}
}

func (r *Runner) runWorkers(
	ctx context.Context,
) error {
	// runCtx is a shared context for all workers.
	// Fail-fast: the first worker returning an error calls cancel(),
	// causing all other workers to stop via ctx.Done().
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	workerSettings := r.enabledProtocolSettings()
	if len(workerSettings) == 0 {
		return errors.New("no protocol is enabled in server configuration")
	}
	errCh := make(chan error, len(workerSettings))
	var wg sync.WaitGroup
	for _, ws := range workerSettings {
		wg.Go(func() {
			if rErr := r.route(runCtx, ws); rErr != nil {
				cancel()
				errCh <- fmt.Errorf("%s worker failed: %w", ws.Protocol, rErr)
			}
		})
	}

	wg.Wait()
	close(errCh)

	//aggregate errors into one error
	errs := make([]error, 0)
	for workerErr := range errCh {
		if workerErr != nil {
			errs = append(errs, fmt.Errorf("worker err: %w", workerErr))
		}
	}
	return errors.Join(errs...)
}

func (r *Runner) enabledProtocolSettings() []settings.Settings {
	enabledProtocolSettings := make([]settings.Settings, 0)
	cfg := r.deps.Configuration()
	if cfg.EnableTCP {
		enabledProtocolSettings = append(enabledProtocolSettings, cfg.TCPSettings)
	}
	if cfg.EnableUDP {
		enabledProtocolSettings = append(enabledProtocolSettings, cfg.UDPSettings)
	}
	if cfg.EnableWS {
		enabledProtocolSettings = append(enabledProtocolSettings, cfg.WSSettings)
	}

	return enabledProtocolSettings
}

func (r *Runner) route(
	ctx context.Context,
	settings settings.Settings,
) error {
	tun, tunErr := r.deps.TunManager().CreateTunDevice(settings)
	if tunErr != nil {
		return fmt.Errorf("error creating tun device: %w", tunErr)
	}

	worker, workerErr := r.workerFactory.CreateWorker(ctx, tun, settings)
	if workerErr != nil {
		return fmt.Errorf("error creating worker: %w", workerErr)
	}

	router := r.routerFactory.CreateRouter(worker)
	routingErr := router.RouteTraffic(ctx)
	if routingErr != nil {
		return fmt.Errorf("error routing traffic: %w", routingErr)
	}

	return nil
}
