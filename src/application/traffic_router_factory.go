package application

import (
	"context"
	"tungo/application/network/tun"
)

type TrafficRouterFactory interface {
	CreateRouter(ctx context.Context,
		connectionFactory ConnectionFactory,
		tunFactory tun.ClientManager,
		workerFactory ClientWorkerFactory,
	) (TrafficRouter, ConnectionAdapter, tun.Device, error)
}

type ServerTrafficRouterFactory interface {
	CreateRouter(worker tun.Worker) TrafficRouter
}
