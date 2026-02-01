package client_factory

import (
	"context"
	"log"
	"tungo/application/network/connection"
	application "tungo/application/network/routing"
	"tungo/application/network/routing/tun"
	implementation "tungo/infrastructure/tunnel"
)

type RouterFactory struct {
}

func NewRouterFactory() connection.TrafficRouterFactory {
	return &RouterFactory{}
}

func (u *RouterFactory) CreateRouter(
	ctx context.Context,
	connectionFactory connection.Factory,
	tunManager tun.ClientManager,
	workerFactory connection.ClientWorkerFactory,
) (application.Router, connection.Transport, tun.Device, error) {
	conn, cryptographyService, controller, connErr := connectionFactory.EstablishConnection(ctx)
	if connErr != nil {
		return nil, nil, nil, connErr
	}

	device, deviceErr := tunManager.CreateDevice()
	if deviceErr != nil {
		log.Printf("failed to create TUN device: %s", deviceErr)
		return nil, nil, nil, deviceErr
	}

	worker, workerErr := workerFactory.CreateWorker(ctx, conn, device, cryptographyService, controller)
	if workerErr != nil {
		return nil, nil, nil, workerErr
	}

	return implementation.NewRouter(worker), conn, device, nil
}
