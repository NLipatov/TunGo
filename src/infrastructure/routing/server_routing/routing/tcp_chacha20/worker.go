package tcp_chacha20

import (
	"context"
	"tungo/application"
)

type TcpTunWorker struct {
	ctx              context.Context
	tunHandler       application.TunHandler
	transportHandler application.TransportHandler
}

func NewTcpTunWorker(
	ctx context.Context,
	tunHandler application.TunHandler,
	transportHandler application.TransportHandler,
) application.TunWorker {
	return &TcpTunWorker{
		ctx:              ctx,
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
