package factory

import (
	"context"
	"net"
	"tungo/application/listeners"
	server2 "tungo/infrastructure/network/ws/server"
)

type ListenerFactory struct {
}

func NewListenerFactory() *ListenerFactory {
	return &ListenerFactory{}
}

func (l *ListenerFactory) BuildDefaultListener(
	ctx context.Context,
	listener net.Listener,
) (listeners.TcpListener, error) {
	queue := make(chan net.Conn, 1024)
	serverFactory := newDefaultServerFactory()
	server, srvErr := serverFactory.NewServer(ctx, listener, queue)
	if srvErr != nil {
		return nil, srvErr
	}
	return server2.NewListener(ctx, server, queue)
}
