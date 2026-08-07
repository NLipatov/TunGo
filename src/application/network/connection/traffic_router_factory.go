package connection

import (
	"context"
	"tungo/application/network/routing"
	"tungo/application/network/routing/tun"
)

type TrafficRouterFactory interface {
	CreateRouter(ctx context.Context,
		connectionFactory Factory,
		tunFactory tun.ClientManager,
		workerFactory ClientWorkerFactory,
	) (routing.Router, Transport, tun.Device, error)
}

type ServerTrafficRouterFactory interface {
	CreateRouter(endpoints routing.Endpoints) routing.Router
}
