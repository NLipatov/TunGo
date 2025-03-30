package application

import "context"

type TrafficRouterFactory interface {
	CreateRouter(ctx context.Context,
		connectionFactory ConnectionFactory,
		tunFactory TunManager,
		workerFactory TunWorkerFactory,
	) (TrafficRouter, error)
}
