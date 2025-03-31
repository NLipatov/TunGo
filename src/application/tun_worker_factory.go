package application

import (
	"io"
	"net"
)

type TunWorkerFactory interface {
	CreateWorker(conn net.Conn, tun io.ReadWriteCloser, cryptographyService CryptographyService) (TunWorker, error)
}
