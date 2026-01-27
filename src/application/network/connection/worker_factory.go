package connection

import (
	"context"
	"io"
	"tungo/application/network/rekey"
	"tungo/application/network/routing"
	"tungo/infrastructure/settings"
)

type ClientWorkerFactory interface {
	CreateWorker(
		ctx context.Context,
		conn Transport,
		tun io.ReadWriteCloser,
		cryptographyService Crypto,
		controller *rekey.Controller,
	) (routing.Worker, error)
}

type ServerWorkerFactory interface {
	CreateWorker(
		ctx context.Context,
		tun io.ReadWriteCloser,
		workerSettings settings.Settings,
	) (routing.Worker, error)
}
