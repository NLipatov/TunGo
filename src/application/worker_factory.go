package application

import (
	"context"
	"io"
	"net"
)

type WorkerFactory interface {
	CreateWorker(ctx context.Context, conn net.Conn, tun io.ReadWriteCloser, cryptographyService CryptographyService) (TunWorker, error)
}

type ServerWorkerFactory interface {
	CreateWorker(ctx context.Context, tun io.ReadWriteCloser) (TunWorker, error)
}
