package application

import (
	"context"
)

type TrafficRouterFactory interface {
	CreateRouter(ctx context.Context,
		connectionFactory ConnectionFactory,
		tunFactory ClientTunManager,
		workerFactory ClientWorkerFactory,
	) (TrafficRouter, ConnectionAdapter, TunDevice, error)
}

type ServerTrafficRouterFactory interface {
	CreateRouter(worker TunWorker) TrafficRouter
}
