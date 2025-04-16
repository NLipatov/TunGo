package application

import (
	"context"
	"io"
	"net"
)

type TunWorkerFactory interface {
	CreateWorker(ctx context.Context, conn net.Conn, tun io.ReadWriteCloser, cryptographyService CryptographyService) (TunWorker, error)
}
