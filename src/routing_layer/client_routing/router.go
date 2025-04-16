package client_routing

import (
	"context"
	"golang.org/x/sync/errgroup"
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
	errGroup, ctx := errgroup.WithContext(ctx)

	// TUN -> Transport
	errGroup.Go(func() error {
		err := r.worker.HandleTun()
		return err
	})

	// Transport -> TUN
	errGroup.Go(func() error {
		err := r.worker.HandleTransport()
		return err
	})

	return errGroup.Wait()
}
