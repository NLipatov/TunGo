package tunnel

import (
	"context"
	"tungo/application/network/routing"
)

type Router struct {
	endpoints routing.Endpoints
}

func NewRouter(endpoints routing.Endpoints) routing.Router {
	return &Router{endpoints: endpoints}
}

func (r *Router) RouteTraffic(ctx context.Context) error {
	routingErr := make(chan error, 2)

	go func() {
		routingErr <- r.endpoints.RunTun()
	}()

	go func() {
		routingErr <- r.endpoints.RunTransport()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-routingErr:
		return err
	}
}
