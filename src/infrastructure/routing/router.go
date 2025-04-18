package routing

import (
	"context"
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
