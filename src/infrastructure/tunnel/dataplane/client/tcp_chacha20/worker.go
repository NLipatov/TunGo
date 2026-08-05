package tcp_chacha20

import (
	"tungo/application/network/routing"
	"tungo/application/network/routing/transport"
	"tungo/application/network/routing/tun"
)

type TcpTunWorker struct {
	tunHandler       tun.Handler
	transportHandler transport.Handler
}

func NewTcpTunWorker(
	tunHandler tun.Handler,
	transportHandler transport.Handler,
) routing.Worker {
	return &TcpTunWorker{
		tunHandler:       tunHandler,
		transportHandler: transportHandler,
	}
}

func (w *TcpTunWorker) HandleTun() error {
	return w.tunHandler.HandleTun()
}

func (w *TcpTunWorker) HandleTransport() error {
	return w.transportHandler.HandleTransport()
}
