package factory

import (
	"context"
	"log"
	"tungo/application"
	"tungo/infrastructure/routing"
)

type RouterFactory struct {
}

func NewRouterFactory() application.TrafficRouterFactory {
	return &RouterFactory{}
}

func (u *RouterFactory) CreateRouter(
	ctx context.Context,
	connectionFactory application.ConnectionFactory,
	tunFactory application.TunManager,
	workerFactory application.WorkerFactory,
) (application.TrafficRouter, application.ConnectionAdapter, application.TunDevice, error) {
	conn, cryptographyService, connErr := connectionFactory.EstablishConnection(ctx)
	if connErr != nil {
		return nil, nil, nil, connErr
	}

	tun, tunErr := tunFactory.CreateTunDevice()
	if tunErr != nil {
		log.Printf("failed to create tun: %s", tunErr)
		return nil, nil, nil, tunErr
	}

	worker, workerErr := workerFactory.CreateWorker(ctx, conn, tun, cryptographyService)
	if workerErr != nil {
		return nil, nil, nil, workerErr
	}

	return routing.NewRouter(worker), conn, tun, nil
}
