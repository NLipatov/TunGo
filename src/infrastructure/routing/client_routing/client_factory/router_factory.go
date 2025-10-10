package client_factory

import (
	"context"
	"log"
	"tungo/application"
	"tungo/application/network/tun"
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
	tunManager tun.ClientManager,
	workerFactory application.ClientWorkerFactory,
) (application.TrafficRouter, application.ConnectionAdapter, tun.Device, error) {
	conn, cryptographyService, connErr := connectionFactory.EstablishConnection(ctx)
	if connErr != nil {
		return nil, nil, nil, connErr
	}

	device, deviceErr := tunManager.CreateDevice()
	if deviceErr != nil {
		log.Printf("failed to create TUN device: %s", deviceErr)
		return nil, nil, nil, deviceErr
	}

	worker, workerErr := workerFactory.CreateWorker(ctx, conn, device, cryptographyService)
	if workerErr != nil {
		return nil, nil, nil, workerErr
	}

	return routing.NewRouter(worker), conn, device, nil
}
