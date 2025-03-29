package client_routing

import (
	"context"
	"errors"
	"log"
	"sync"
	"tungo/application"
)

type Router struct {
	worker application.TunWorker
}

func NewRouter(worker application.TunWorker) application.TrafficRouter {
	return &Router{
		worker: worker,
	}
}

func (r *Router) RouteTraffic(ctx context.Context) {
	routingCtx, routingCancel := context.WithCancel(ctx)
	defer routingCancel()

	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> Transport
	go func() {
		defer wg.Done()
		if err := r.worker.HandleTun(routingCtx, routingCancel); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("TUN -> Transport error: %v", err)
			routingCancel()
			return
		}
	}()

	// Transport -> TUN
	go func() {
		defer wg.Done()
		if err := r.worker.HandleTransport(routingCtx, routingCancel); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("Transport -> TUN error: %v", err)
			routingCancel()
			return
		}
	}()

	wg.Wait()
}
