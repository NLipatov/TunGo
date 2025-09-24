package factory

import (
	"context"
	"net"
	"tungo/application/listeners"
	wsServer "tungo/infrastructure/network/ws/server"
	"tungo/infrastructure/network/ws/server/contracts"
)

type ListenerFactory struct {
	serverFactory contracts.ServerFactory
}

func NewDefaultListenerFactory() *ListenerFactory {
	return &ListenerFactory{
		serverFactory: newDefaultServerFactory(),
	}
}

func NewListenerFactory(serverFactory contracts.ServerFactory) *ListenerFactory {
	return &ListenerFactory{
		serverFactory: serverFactory,
	}
}

func (lf *ListenerFactory) NewListener(
	ctx context.Context,
	listener net.Listener,
) (listeners.TcpListener, error) {
	queue := make(chan net.Conn, 1024)
	server, srvErr := lf.serverFactory.NewServer(ctx, listener, queue)
	if srvErr != nil {
		return nil, srvErr
	}
	return wsServer.NewListener(ctx, server, queue)
}
