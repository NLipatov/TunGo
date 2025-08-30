package application

import (
	"context"
	"io"
	"tungo/infrastructure/settings"
)

type ClientWorkerFactory interface {
	CreateWorker(
		ctx context.Context,
		conn ConnectionAdapter,
		tun io.ReadWriteCloser,
		cryptographyService CryptographyService,
	) (TunWorker, error)
}

type ServerWorkerFactory interface {
	CreateWorker(
		ctx context.Context,
		tun io.ReadWriteCloser,
		workerSettings settings.Settings,
	) (TunWorker, error)
}
