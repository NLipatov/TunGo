package tun_server

import (
	"context"
	"fmt"
	"io"
	"tungo/application"
	"tungo/infrastructure/routing/server_routing/routing/tcp_chacha20"
	"tungo/infrastructure/routing/server_routing/routing/udp_chacha20"
	"tungo/infrastructure/routing/server_routing/session_management"
	"tungo/infrastructure/settings"
)

type ServerWorkerFactory struct {
	settings settings.Settings
}

func NewServerWorkerFactory(settings settings.Settings) application.ServerWorkerFactory {
	return &ServerWorkerFactory{
		settings: settings,
	}
}

func (s ServerWorkerFactory) CreateWorker(ctx context.Context, tun io.ReadWriteCloser) (application.TunWorker, error) {
	switch s.settings.Protocol {
	case settings.TCP:
		sessionManager := session_management.NewDefaultWorkerSessionManager[tcp_chacha20.Session]()
		tunHandler := tcp_chacha20.NewTunHandler(ctx, tun, sessionManager)
		transportHandler := tcp_chacha20.NewTransportHandler(ctx, s.settings, tun, sessionManager)
		return tcp_chacha20.NewTcpTunWorker(ctx, tunHandler, transportHandler), nil
	case settings.UDP:
		return udp_chacha20.NewUdpTunWorker(ctx, tun, s.settings), nil
	default:
		return nil, fmt.Errorf("protocol %v not supported", s.settings.Protocol)
	}
}
