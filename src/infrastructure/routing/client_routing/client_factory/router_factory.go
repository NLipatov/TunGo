package client_factory

import (
	"context"
	"log"
	"time"

	"tungo/application"
	"tungo/infrastructure/routing"
	"tungo/infrastructure/settings"
)

type RouterFactory struct {
}

func NewRouterFactory() application.TrafficRouterFactory {
	return &RouterFactory{}
}

func (u *RouterFactory) CreateRouter(
	ctx context.Context,
	connectionFactory application.ConnectionFactory,
	tunFactory application.ClientTunManager,
	workerFactory application.ClientWorkerFactory,
) (application.TrafficRouter, application.ConnectionAdapter, application.TunDevice, error) {
	conn, cryptographyService, connErr := connectionFactory.EstablishConnection(ctx)
	if connErr != nil {
		return nil, nil, nil, connErr
	}

	// Run MTU discovery before creating the TUN device.
	prober := application.NewConnMTUProber(conn, cryptographyService)
	if mtu, err := application.DiscoverMTU(prober, settings.MTU, 1500, 200*time.Millisecond); err != nil {
		log.Printf("mtu discovery failed: %v", err)
	} else {
		log.Printf("mtu discovered: %d", mtu)
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
