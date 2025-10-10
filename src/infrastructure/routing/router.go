package routing

import (
	"context"
	"tungo/application"
	"tungo/application/network/tun"
)

type Router struct {
	worker tun.Worker
}

func NewRouter(worker tun.Worker) application.TrafficRouter {
	return &Router{
		worker: worker,
	}
}

func (r *Router) RouteTraffic(ctx context.Context) error {
	routingErr := make(chan error, 2)

	go func() {
		routingErr <- r.worker.HandleTun()
	}()

	go func() {
		routingErr <- r.worker.HandleTransport()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-routingErr:
		return err
	}
}
