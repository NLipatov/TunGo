package tcp_chacha20

import (
	"tungo/application"
)

type TcpTunWorker struct {
	tunHandler       application.TunHandler
	transportHandler application.TransportHandler
}

func NewTcpTunWorker(
	tunHandler application.TunHandler,
	transportHandler application.TransportHandler,
) application.TunWorker {
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
