package tun_server

import "net"

type tcpListenerFactory interface {
	listenTCP(addr string) (net.Listener, error)
}

type defaultTcpListenerFactory struct {
}

func newDefaultTcpListenerFactory() tcpListenerFactory {
	return &defaultTcpListenerFactory{}
}

func (factory defaultTcpListenerFactory) listenTCP(addr string) (net.Listener, error) {
	return net.Listen("tcp", addr)
}
