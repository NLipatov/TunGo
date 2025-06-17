package tun_server

import (
	"tungo/application"
	"tungo/infrastructure/network"
)

type socketFactory interface {
	newSocket(ip, port string) (application.Socket, error)
}

type defaultSocketFactory struct {
}

func newDefaultSocketFactory() socketFactory {
	return &defaultSocketFactory{}
}

func (s *defaultSocketFactory) newSocket(ip, port string) (application.Socket, error) {
	return network.NewSocket(ip, port)
}
