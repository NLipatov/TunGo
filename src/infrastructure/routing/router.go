package routing

import (
	"context"
	"tungo/application/network/routing"
)

type Router struct {
	worker routing.Worker
}

func NewRouter(worker routing.Worker) routing.Router {
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
