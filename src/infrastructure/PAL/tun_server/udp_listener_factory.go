package tun_server

import (
	"tungo/application"
	"tungo/infrastructure/listeners/udp_listener"
)

type udpListenerFactory interface {
	listenUDP(sock application.Socket) udp_listener.Listener
}

type defaultUdpListenerFactory struct {
}

func newDefaultUdpListenerFactory() udpListenerFactory {
	return &defaultUdpListenerFactory{}
}

func (d defaultUdpListenerFactory) listenUDP(sock application.Socket) udp_listener.Listener {
	return udp_listener.NewUdpListener(sock)
}
