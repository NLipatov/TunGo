package udp_chacha20

import (
	"tungo/application"
)

type UdpTunWorker struct {
	tunHandler       application.TunHandler
	transportHandler application.TransportHandler
}

func NewUdpTunWorker(
	tunHandler application.TunHandler,
	transportHandler application.TransportHandler,
) application.TunWorker {
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
