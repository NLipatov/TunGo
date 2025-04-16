package presentation

import (
	"context"
	"errors"
	"log"
	"time"
	"tungo/application"
	"tungo/infrastructure/routing_layer/client_routing/factory"
)

type ClientRunner struct {
	deps          ClientAppDependencies
	routerFactory application.TrafficRouterFactory
}

func NewClientRunner(deps ClientAppDependencies) *ClientRunner {
	return &ClientRunner{
		deps:          deps,
		routerFactory: factory.NewRouterFactory(),
	}
}

func (r *ClientRunner) Run(ctx context.Context) {
	for ctx.Err() == nil {
		if err := r.deps.TunManager().DisposeTunDevices(); err != nil {
			log.Printf("error disposing tun devices: %s", err)
		}

		router, conn, tun, err := r.routerFactory.
			CreateRouter(ctx, r.deps.ConnectionFactory(), r.deps.TunManager(), r.deps.WorkerFactory())
		if err != nil {
			log.Printf("failed to create router: %s", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		log.Printf("tunneling traffic via tun device")

		go func() {
			<-ctx.Done() //blocks until context is cancelled
			_ = conn.Close()
			_ = tun.Close()
		}()

		if routeTrafficErr := router.RouteTraffic(ctx); routeTrafficErr != nil {
			if errors.Is(routeTrafficErr, context.Canceled) {
				break
			}
			log.Printf("routing error: %s", routeTrafficErr)
		}
	}

	if err := r.deps.TunManager().DisposeTunDevices(); err != nil {
		log.Printf("error disposing tun devices on exit: %s", err)
	}
}
