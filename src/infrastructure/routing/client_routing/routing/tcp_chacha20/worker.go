package tcp_chacha20

import (
	"context"
	"tungo/application"
)

type TcpTunWorker struct {
	ctx                 context.Context
	cryptographyService application.CryptographyService
	tunHandler          application.TunHandler
	transportHandler    application.TransportHandler
}

func NewTcpTunWorker(
	ctx context.Context,
	tunHandler application.TunHandler,
	transportHandler application.TransportHandler,
	cryptographyService application.CryptographyService,
) application.TunWorker {
	return &TcpTunWorker{
		ctx:                 ctx,
		cryptographyService: cryptographyService,
		tunHandler:          tunHandler,
		transportHandler:    transportHandler,
	}
}

func (w *TcpTunWorker) HandleTun() error {
	return w.tunHandler.HandleTun()
}

func (w *TcpTunWorker) HandleTransport() error {
	return w.transportHandler.HandleTransport()
}
