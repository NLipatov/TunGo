package routing

import (
	"context"
	"golang.org/x/sync/errgroup"
	"tungo/application"
)

type ServerRouter struct {
	worker application.TunWorker
}

func NewServerRouter(worker application.TunWorker) application.TrafficRouter {
	return &ServerRouter{
		worker: worker,
	}
}

func (r *ServerRouter) RouteTraffic(ctx context.Context) error {
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
