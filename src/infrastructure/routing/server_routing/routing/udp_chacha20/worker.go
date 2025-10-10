package udp_chacha20

import (
	"tungo/application/network/routing"
	"tungo/application/network/routing/transport"
	"tungo/application/network/routing/tun"
)

type UdpTunWorker struct {
	tunHandler       tun.Handler
	transportHandler transport.Handler
}

func NewUdpTunWorker(
	tunHandler tun.Handler,
	transportHandler transport.Handler,
) routing.Worker {
	return &UdpTunWorker{
		tunHandler:       tunHandler,
		transportHandler: transportHandler,
	}
}

func (u *UdpTunWorker) HandleTun() error {
	return u.tunHandler.HandleTun()
}

func (u *UdpTunWorker) HandleTransport() error {
	return u.transportHandler.HandleTransport()
}
