package tun_server

import (
	"context"
	"fmt"
	"io"

	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/network"
	"tungo/infrastructure/routing/server_routing/routing/tcp_chacha20"
	"tungo/infrastructure/routing/server_routing/routing/udp_chacha20"
	"tungo/infrastructure/routing/server_routing/session_management"
	"tungo/infrastructure/settings"
)

type ServerWorkerFactory struct {
	settings      settings.Settings
	socketFactory socketFactory
	tcpFactory    tcpListenerFactory
	udpFactory    udpListenerFactory
	loggerFactory loggerFactory
}

func NewServerWorkerFactory(settings settings.Settings) application.ServerWorkerFactory {
	return &ServerWorkerFactory{
		settings:      settings,
		socketFactory: newDefaultSocketFactory(),
		tcpFactory:    newDefaultTcpListenerFactory(),
		udpFactory:    newDefaultUdpListenerFactory(),
		loggerFactory: newDefaultLoggerFactory(),
	}
}

func NewTestServerWorkerFactory(
	settings settings.Settings,
	socketFactory socketFactory,
	tcpFactory tcpListenerFactory,
	udpFactory udpListenerFactory,
	loggerFactory loggerFactory,
) application.ServerWorkerFactory {
	return &ServerWorkerFactory{
		settings:      settings,
		socketFactory: socketFactory,
		tcpFactory:    tcpFactory,
		udpFactory:    udpFactory,
		loggerFactory: loggerFactory,
	}
}

func (s *ServerWorkerFactory) CreateWorker(ctx context.Context, tun io.ReadWriteCloser) (application.TunWorker, error) {
	switch s.settings.Protocol {
	case settings.TCP:
		return s.createTCPWorker(ctx, tun)
	case settings.UDP:
		return s.createUDPWorker(ctx, tun)
	default:
		return nil, fmt.Errorf("protocol %v not supported", s.settings.Protocol)
	}
}

func (s *ServerWorkerFactory) createTCPWorker(ctx context.Context, tun io.ReadWriteCloser) (application.TunWorker, error) {
	// session managers, handlers…
	sm := session_management.NewConcurrentManager(
		session_management.NewDefaultWorkerSessionManager[tcp_chacha20.Session](),
	)
	th := tcp_chacha20.NewTunHandler(
		ctx,
		chacha20.NewTcpReader(tun),
		chacha20.NewDefaultTCPEncoder(),
		network.NewIPV4HeaderParser(),
		sm,
	)

	// now the injected factories:
	sock, err := s.socketFactory.newSocket(s.settings.ConnectionIP, s.settings.Port)
	if err != nil {
		return nil, err
	}
	listener, err := s.tcpFactory.listenTCP(sock.StringAddr())
	if err != nil {
		return nil, fmt.Errorf("failed to listen TCP: %w", err)
	}

	tr := tcp_chacha20.NewTransportHandler(
		ctx,
		s.settings,
		tun,
		listener,
		sm,
		s.loggerFactory.newLogger(),
	)
	return tcp_chacha20.NewTcpTunWorker(th, tr), nil
}

func (s *ServerWorkerFactory) createUDPWorker(ctx context.Context, tun io.ReadWriteCloser) (application.TunWorker, error) {
	sm := session_management.NewConcurrentManager(
		session_management.NewDefaultWorkerSessionManager[udp_chacha20.Session](),
	)
	th := udp_chacha20.NewTunHandler(
		ctx,
		chacha20.NewUdpReader(tun),
		network.NewIPV4HeaderParser(),
		sm,
	)

	sock, err := s.socketFactory.newSocket(s.settings.ConnectionIP, s.settings.Port)
	if err != nil {
		return nil, err
	}
	ul := s.udpFactory.listenUDP(sock)

	tr := udp_chacha20.NewTransportHandler(
		ctx,
		s.settings,
		tun,
		ul,
		sm,
		s.loggerFactory.newLogger(),
	)
	return udp_chacha20.NewUdpTunWorker(th, tr), nil
}
