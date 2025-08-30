package server

import (
	"context"
	"fmt"
	"log"
	"sync"
	"tungo/infrastructure/PAL/tun_server"
	"tungo/infrastructure/routing"
	"tungo/infrastructure/settings"

	"golang.org/x/sync/errgroup"
)

type Runner struct {
	deps AppDependencies
}

func NewRunner(deps AppDependencies) *Runner {
	return &Runner{
		deps: deps,
	}
}

func (r *Runner) Run(ctx context.Context) {
	err := r.deps.KeyManager().PrepareKeys()
	if err != nil {
		log.Fatalf("failed to generate ed25519 keys: %s", err)
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

	r.runWorkers(ctx)
}

func (r *Runner) runWorkers(ctx context.Context) {
	// runCtx is a shared context for all workers.
	// Fail-fast: the first worker returning an error calls cancel(),
	// causing all other workers to stop via ctx.Done().
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, 3)
	var wg sync.WaitGroup
	if r.deps.Configuration().EnableTCP {
		wg.Go(func() {
			if rErr := r.route(runCtx, r.deps.Configuration().TCPSettings); rErr != nil {
				cancel()
				errCh <- rErr
			}
		})
	}

	if r.deps.Configuration().EnableUDP {
		wg.Go(func() {
			if rErr := r.route(runCtx, r.deps.Configuration().UDPSettings); rErr != nil {
				cancel()
				errCh <- rErr
			}
		})
	}

	if r.deps.Configuration().EnableWS {
		wg.Go(func() {
			if rErr := r.route(runCtx, r.deps.Configuration().WSSettings); rErr != nil {
				cancel()
				errCh <- rErr
			}
		})
	}

	wg.Wait()
	close(errCh)
	for workerErr := range errCh {
		if workerErr != nil {
			log.Printf("worker err: %s", workerErr)
		}
	}
}

func (r *Runner) route(ctx context.Context, settings settings.Settings) error {
	workerFactory := tun_server.NewServerWorkerFactory(settings, r.deps.ConfigurationManager())

	tun, tunErr := r.deps.TunManager().CreateTunDevice(settings)
	if tunErr != nil {
		return fmt.Errorf("error creating tun device: %s", tunErr)
	}

	worker, workerErr := workerFactory.CreateWorker(ctx, tun)
	if workerErr != nil {
		return fmt.Errorf("error creating worker: %s", workerErr)
	}

	router := routing.NewRouter(worker)

	routingErr := router.RouteTraffic(ctx)
	if routingErr != nil {
		return fmt.Errorf("error routing traffic: %s", routingErr)
	}

	return nil
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
