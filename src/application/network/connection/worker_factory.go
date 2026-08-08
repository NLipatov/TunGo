package connection

import (
	"context"
	"io"
	"tungo/application/configuration/settings"
	"tungo/application/network/routing"
)

type ClientWorkerFactory interface {
	CreateWorker(
		ctx context.Context,
		conn Transport,
		tun io.ReadWriteCloser,
		cryptographyService Crypto,
		controller RekeyController,
	) (routing.Endpoints, error)
}

type ServerWorkerFactory interface {
	CreateWorker(
		ctx context.Context,
		tun io.ReadWriteCloser,
		workerSettings settings.Settings,
	) (routing.Endpoints, error)
}
