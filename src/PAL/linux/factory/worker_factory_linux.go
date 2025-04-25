package factory

import (
	"context"
	"fmt"
	"io"
	"tungo/application"
	"tungo/infrastructure/routing/server_routing/routing/tcp_chacha20"
	"tungo/infrastructure/routing/server_routing/routing/udp_chacha20"
	"tungo/settings"
)

type ServerWorkerFactory struct {
	settings settings.ConnectionSettings
}

func NewServerWorkerFactory(settings settings.ConnectionSettings) application.ServerWorkerFactory {
	return &ServerWorkerFactory{
		settings: settings,
	}
}

func (s ServerWorkerFactory) CreateWorker(ctx context.Context, tun io.ReadWriteCloser) (application.TunWorker, error) {
	switch s.settings.Protocol {
	case settings.TCP:
		return tcp_chacha20.NewTcpTunWorker(ctx, tun, s.settings), nil
	case settings.UDP:
		return udp_chacha20.NewUdpTunWorker(ctx, tun, s.settings), nil
	default:
		return nil, fmt.Errorf("protocol %v not supported", s.settings.Protocol)
	}
}
