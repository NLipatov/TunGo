package udp_chacha20

import (
	"tungo/application/network/routing/transport"
	"tungo/application/network/routing/tun"
)

type UdpWorker struct {
	transport transport.Handler
	tun       tun.Handler
}

func NewUdpWorker(
	transport transport.Handler,
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
