package udp_chacha20

import (
	"tungo/application"
	"tungo/application/network/tun"
)

type UdpTunWorker struct {
	tunHandler       tun.Handler
	transportHandler application.TransportHandler
}

func NewUdpTunWorker(
	tunHandler tun.Handler,
	transportHandler application.TransportHandler,
) tun.Worker {
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
