package application

import (
	"context"
	"io"
)

type ClientWorkerFactory interface {
	CreateWorker(ctx context.Context, conn ConnectionAdapter, tun io.ReadWriteCloser, cryptographyService CryptographyService) (TunWorker, error)
}

type ServerWorkerFactory interface {
	CreateWorker(ctx context.Context, tun io.ReadWriteCloser) (TunWorker, error)
}
