package tun_server

import (
	"context"
	"fmt"
	"io"
	"tungo/application"
	"tungo/infrastructure/routing/server_routing/routing/tcp_chacha20"
	"tungo/infrastructure/routing/server_routing/routing/udp_chacha20"
	settings2 "tungo/infrastructure/settings"
)

type ServerWorkerFactory struct {
	settings settings2.Settings
}

func NewServerWorkerFactory(settings settings2.Settings) application.ServerWorkerFactory {
	return &ServerWorkerFactory{
		settings: settings,
	}
}

func (s ServerWorkerFactory) CreateWorker(ctx context.Context, tun io.ReadWriteCloser) (application.TunWorker, error) {
	switch s.settings.Protocol {
	case settings2.TCP:
		return tcp_chacha20.NewTcpTunWorker(ctx, tun, s.settings), nil
	case settings2.UDP:
		return udp_chacha20.NewUdpTunWorker(ctx, tun, s.settings), nil
	default:
		return nil, fmt.Errorf("protocol %v not supported", s.settings.Protocol)
	}
}
