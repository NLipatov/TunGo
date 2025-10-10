package udp_chacha20

import (
	"tungo/application"
	"tungo/application/network/tun"
)

type UdpWorker struct {
	transport application.TransportHandler
	tun       tun.Handler
}

func NewUdpWorker(
	transport application.TransportHandler,
	tun tun.Handler,
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
