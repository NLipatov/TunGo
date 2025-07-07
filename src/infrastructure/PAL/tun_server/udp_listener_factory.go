package tun_server

import (
	"tungo/application"
	"tungo/infrastructure/routing/server_routing/routing/udp_chacha20"
)

type udpListenerFactory interface {
	listenUDP(sock application.Socket) application.Listener
}

type defaultUdpListenerFactory struct {
}

func newDefaultUdpListenerFactory() udpListenerFactory {
	return &defaultUdpListenerFactory{}
}

func (d defaultUdpListenerFactory) listenUDP(sock application.Socket) application.Listener {
	return udp_chacha20.NewListener(sock)
}
