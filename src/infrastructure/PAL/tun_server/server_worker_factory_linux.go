package tun_server

import (
	"context"
	"fmt"
	"io"
	"net"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/listeners/udp_listener"
	"tungo/infrastructure/logging"
	"tungo/infrastructure/network"
	"tungo/infrastructure/routing/server_routing/routing/tcp_chacha20"
	"tungo/infrastructure/routing/server_routing/routing/udp_chacha20"
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
		return s.createTCPWorker(ctx, tun)
	case settings.UDP:
		return s.createUDPWorker(ctx, tun)
	default:
		return nil, fmt.Errorf("protocol %v not supported", s.settings.Protocol)
	}
}

func (s ServerWorkerFactory) createTCPWorker(ctx context.Context, tun io.ReadWriteCloser) (application.TunWorker, error) {
	logger := logging.NewLogLogger()
	smFactory := newSessionManagerFactory[tcp_chacha20.Session]()
	concurrentSessionManager := smFactory.createConcurrentManager()
	tunHandler := tcp_chacha20.NewTunHandler(ctx,
		chacha20.NewTcpReader(tun),
		chacha20.NewDefaultTCPEncoder(),
		network.NewIPV4HeaderParser(),
		concurrentSessionManager)
	listener, err := net.Listen("tcp", net.JoinHostPort("", s.settings.Port))
	if err != nil {
		return nil, fmt.Errorf("failed to listen on port %s: %v", s.settings.Port, err)
	}
	transportHandler := tcp_chacha20.NewTransportHandler(ctx, s.settings, tun, listener, concurrentSessionManager, logger)
	return tcp_chacha20.NewTcpTunWorker(tunHandler, transportHandler), nil
}

func (s ServerWorkerFactory) createUDPWorker(ctx context.Context, tun io.ReadWriteCloser) (application.TunWorker, error) {
	logger := logging.NewLogLogger()
	smFactory := newSessionManagerFactory[udp_chacha20.Session]()
	concurrentSessionManager := smFactory.createConcurrentManager()
	tunHandler := udp_chacha20.NewTunHandler(ctx,
		chacha20.NewUdpReader(tun),
		network.NewIPV4HeaderParser(),
		concurrentSessionManager)
	socket, socketErr := network.NewSocket(s.settings.ConnectionIP, s.settings.Port)
	if socketErr != nil {
		return nil, socketErr
	}
	transportHandler := udp_chacha20.NewTransportHandler(
		ctx,
		s.settings,
		tun,
		concurrentSessionManager,
		logger,
		udp_listener.NewUdpListener(socket))
	return udp_chacha20.NewUdpTunWorker(tunHandler, transportHandler), nil
}
