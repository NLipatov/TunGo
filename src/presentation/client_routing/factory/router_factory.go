package factory

import (
	"context"
	"log"
	"tungo/application"
	"tungo/presentation/client_routing"
)

type RouterFactory struct {
}

func NewRouterBuilder() application.TrafficRouterFactory {
	return &RouterFactory{}
}

func (u *RouterFactory) CreateRouter(
	ctx context.Context,
	connectionFactory application.ConnectionFactory,
	tunFactory application.TunManager,
	workerFactory application.TunWorkerFactory,
) (application.TrafficRouter, error) {
	conn, cryptographyService, connErr := connectionFactory.EstablishConnection(ctx)
	if connErr != nil {
		return nil, connErr
	}

	tun, tunErr := tunFactory.CreateTunDevice()
	if tunErr != nil {
		log.Printf("failed to create tun: %s", tunErr)
		return nil, tunErr
	}

	worker, workerErr := workerFactory.CreateWorker(conn, tun, cryptographyService)
	if workerErr != nil {
		return nil, workerErr
	}

	return client_routing.NewRouter(worker), nil
}
