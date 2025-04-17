package application

import (
	"context"
)

type TrafficRouterFactory interface {
	CreateRouter(ctx context.Context,
		connectionFactory ConnectionFactory,
		tunFactory TunManager,
		workerFactory WorkerFactory,
	) (TrafficRouter, ConnectionAdapter, TunDevice, error)
}

type ServerTrafficRouterFactory interface {
	CreateRouter(worker TunWorker) TrafficRouter
}
