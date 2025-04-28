package server

import (
	"context"
	"log"
	"sync"
	"tungo/infrastructure/PAL/pal_factory"
	"tungo/infrastructure/routing/server_routing/factory"
	"tungo/settings"
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

	var wg sync.WaitGroup
	if r.deps.Configuration().EnableTCP {
		wg.Add(1)

		connSettings := r.deps.Configuration().TCPSettings
		if err := r.deps.TunManager().DisposeTunDevices(connSettings); err != nil {
			log.Printf("error disposing tun devices: %s", err)
		}

		go func() {
			defer wg.Done()
			routeErr := r.route(ctx, connSettings)
			if routeErr != nil {
				log.Println(routeErr)
			}
		}()
	}

	if r.deps.Configuration().EnableUDP {
		wg.Add(1)

		connSettings := r.deps.Configuration().UDPSettings
		if err := r.deps.TunManager().DisposeTunDevices(connSettings); err != nil {
			log.Printf("error disposing tun devices: %s", err)
		}

		go func() {
			defer wg.Done()
			routeErr := r.route(ctx, connSettings)
			if routeErr != nil {
				log.Println(routeErr)
			}
		}()
	}

	wg.Wait()
}

func (r *Runner) route(ctx context.Context, settings settings.ConnectionSettings) error {
	workerFactory := pal_factory.NewServerWorkerFactory(settings)
	tunFactory := pal_factory.NewServerTunFactory()
	routerFactory := factory.NewServerRouterFactory()

	tun, tunErr := tunFactory.CreateTunDevice(settings)
	if tunErr != nil {
		log.Fatalf("error creating tun device: %s", tunErr)
	}

	worker, workerErr := workerFactory.CreateWorker(ctx, tun)
	if workerErr != nil {
		log.Fatalf("error creating worker: %s", workerErr)
	}

	router := routerFactory.CreateRouter(worker)

	routingErr := router.RouteTraffic(ctx)
	if routingErr != nil {
		log.Fatalf("error routing traffic: %s", routingErr)
	}

	return nil
}
