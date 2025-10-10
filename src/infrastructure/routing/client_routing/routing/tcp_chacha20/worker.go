package tcp_chacha20

import (
	"context"
	"tungo/application"
	"tungo/application/network/tun"
)

type TcpTunWorker struct {
	ctx                 context.Context
	cryptographyService application.CryptographyService
	tunHandler          tun.Handler
	transportHandler    application.TransportHandler
}

func NewTcpTunWorker(
	ctx context.Context,
	tunHandler tun.Handler,
	transportHandler application.TransportHandler,
	cryptographyService application.CryptographyService,
) tun.Worker {
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
