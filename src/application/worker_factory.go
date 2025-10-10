package application

import (
	"context"
	"io"
	"tungo/application/network/tun"
	"tungo/infrastructure/settings"
)

type ClientWorkerFactory interface {
	CreateWorker(
		ctx context.Context,
		conn ConnectionAdapter,
		tun io.ReadWriteCloser,
		cryptographyService CryptographyService,
	) (tun.Worker, error)
}

type ServerWorkerFactory interface {
	CreateWorker(
		ctx context.Context,
		tun io.ReadWriteCloser,
		workerSettings settings.Settings,
	) (tun.Worker, error)
}
