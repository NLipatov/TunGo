package udp_chacha20

import (
	"tungo/application"
)

type UdpWorker struct {
	transport application.TransportHandler
	tun       application.TunHandler
}

func NewUdpWorker(
	transport application.TransportHandler,
	tun application.TunHandler,
) *UdpWorker {
	return &UdpWorker{
		transport: transport,
		tun:       tun,
	}
}

func (w *UdpWorker) HandleTun() error {
	return w.tun.HandleTun()
}

func (w *UdpWorker) HandleTransport() error {
	return w.transport.HandleTransport()
}
