package tcp_chacha20

import (
	"tungo/application"
	"tungo/application/network/tun"
)

type TcpTunWorker struct {
	tunHandler       tun.Handler
	transportHandler application.TransportHandler
}

func NewTcpTunWorker(
	tunHandler tun.Handler,
	transportHandler application.TransportHandler,
) tun.Worker {
	return &TcpTunWorker{
		tunHandler:       tunHandler,
		transportHandler: transportHandler,
	}
}

func (t *TcpTunWorker) HandleTun() error {
	return t.tunHandler.HandleTun()
}

func (t *TcpTunWorker) HandleTransport() error {
	return t.transportHandler.HandleTransport()
}
