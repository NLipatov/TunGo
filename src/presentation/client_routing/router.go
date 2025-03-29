package client_routing

import (
	"context"
	"errors"
	"log"
	"sync"
	"tungo/application"
)

type UnifiedRouter struct {
	worker application.TunWorker
}

func NewUnifiedRouter(worker application.TunWorker) application.TrafficRouter {
	return &UnifiedRouter{
		worker: worker,
	}
}

func (r *UnifiedRouter) RouteTraffic(ctx context.Context) error {
	routingCtx, routingCancel := context.WithCancel(ctx)
	defer routingCancel()

	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> UDP
	go func() {
		defer wg.Done()
		if err := r.worker.HandleTun(routingCtx, routingCancel); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("TUN -> UDP error: %v", err)
			routingCancel()
			return
		}
	}()

	// UDP -> TUN
	go func() {
		defer wg.Done()
		if err := r.worker.HandleConn(routingCtx, routingCancel); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("UDP -> TUN error: %v", err)
			routingCancel()
			return
		}
	}()

	wg.Wait()

	return nil
}
