package tcp_chacha20

import (
	"context"
	"tungo/application/network/connection"
	"tungo/application/network/routing"
	"tungo/application/network/routing/transport"
	"tungo/application/network/routing/tun"
)

type TcpTunWorker struct {
	ctx                 context.Context
	cryptographyService connection.Crypto
	tunHandler          tun.Handler
	transportHandler    transport.Handler
}

func NewTcpTunWorker(
	ctx context.Context,
	tunHandler tun.Handler,
	transportHandler transport.Handler,
	cryptographyService connection.Crypto,
) routing.Worker {
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
