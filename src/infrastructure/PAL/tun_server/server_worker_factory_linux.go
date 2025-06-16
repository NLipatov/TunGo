package tun_server

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/logging"
	"tungo/infrastructure/network"
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
		concurrentSessionManager := session_management.NewConcurrentManager(sessionManager)
		tunHandler := tcp_chacha20.NewTunHandler(ctx,
			chacha20.NewTcpReader(tun),
			chacha20.NewDefaultTCPEncoder(),
			network.NewIPV4HeaderParser(),
			concurrentSessionManager)
		listener, err := net.Listen("tcp", net.JoinHostPort("", s.settings.Port))
		if err != nil {
			log.Printf("failed to listen on port %s: %v", s.settings.Port, err)
		}
		transportHandler := tcp_chacha20.NewTransportHandler(ctx, s.settings, tun, listener, concurrentSessionManager, logging.NewLogLogger())
		return tcp_chacha20.NewTcpTunWorker(tunHandler, transportHandler), nil
	case settings.UDP:
		sessionManager := session_management.NewDefaultWorkerSessionManager[udp_chacha20.Session]()
		concurrentSessionManager := session_management.NewConcurrentManager(sessionManager)
		tunHandler := udp_chacha20.NewTunHandler(ctx,
			tun,
			network.NewIPV4HeaderParser(),
			concurrentSessionManager)
		transportHandler := udp_chacha20.NewTransportHandler(ctx, s.settings, tun, concurrentSessionManager)
		return udp_chacha20.NewUdpTunWorker(tunHandler, transportHandler), nil
	default:
		return nil, fmt.Errorf("protocol %v not supported", s.settings.Protocol)
	}
}
