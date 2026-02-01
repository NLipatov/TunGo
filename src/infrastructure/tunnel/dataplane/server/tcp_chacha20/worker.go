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

func (t *TcpTunWorker) HandleTun() error {
	return t.tunHandler.HandleTun()
}

func (t *TcpTunWorker) HandleTransport() error {
	return t.transportHandler.HandleTransport()
}
