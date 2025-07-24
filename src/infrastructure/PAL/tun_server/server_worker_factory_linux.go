package tun_server

import (
	"context"
	"fmt"
	"io"
	"tungo/application"
	"tungo/infrastructure/PAL/server_configuration"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/network"
	"tungo/infrastructure/routing/server_routing/routing/tcp_chacha20"
	"tungo/infrastructure/routing/server_routing/routing/udp_chacha20"
	"tungo/infrastructure/routing/server_routing/session_management/repository"
	"tungo/infrastructure/routing/server_routing/session_management/repository/wrappers"
	"tungo/infrastructure/settings"
)

type ServerWorkerFactory struct {
	settings             settings.Settings
	socketFactory        socketFactory
	tcpFactory           tcpListenerFactory
	udpFactory           udpListenerFactory
	loggerFactory        loggerFactory
	configurationManager server_configuration.ServerConfigurationManager
}

func NewServerWorkerFactory(settings settings.Settings, manager server_configuration.ServerConfigurationManager) application.ServerWorkerFactory {
	return &ServerWorkerFactory{
		settings:             settings,
		socketFactory:        newDefaultSocketFactory(),
		tcpFactory:           newDefaultTcpListenerFactory(),
		udpFactory:           newDefaultUdpListenerFactory(),
		loggerFactory:        newDefaultLoggerFactory(),
		configurationManager: manager,
	}
}

func NewTestServerWorkerFactory(
	settings settings.Settings,
	socketFactory socketFactory,
	tcpFactory tcpListenerFactory,
	udpFactory udpListenerFactory,
	loggerFactory loggerFactory,
	manager server_configuration.ServerConfigurationManager,
) application.ServerWorkerFactory {
	return &ServerWorkerFactory{
		settings:             settings,
		socketFactory:        socketFactory,
		tcpFactory:           tcpFactory,
		udpFactory:           udpFactory,
		loggerFactory:        loggerFactory,
		configurationManager: manager,
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
	sessionManager := repository.NewDefaultWorkerSessionManager[application.Session]()
	concurrentSessionManager := wrappers.NewConcurrentManager(sessionManager)

	th := tcp_chacha20.NewTunHandler(
		ctx,
		chacha20.NewTcpReader(tun),
		chacha20.NewDefaultTCPEncoder(),
		network.NewIPV4HeaderParser(),
		concurrentSessionManager,
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

	conf, confErr := s.configurationManager.Configuration()
	if confErr != nil {
		return nil, confErr
	}

	tr := tcp_chacha20.NewTransportHandler(
		ctx,
		s.settings,
		tun,
		listener,
		sessionManager,
		s.loggerFactory.newLogger(),
		NewHandshakeFactory(*conf),
		chacha20.NewTcpSessionBuilder(),
	)
	return tcp_chacha20.NewTcpTunWorker(th, tr), nil
}

func (s *ServerWorkerFactory) createUDPWorker(ctx context.Context, tun io.ReadWriteCloser) (application.TunWorker, error) {
	sessionManager := repository.NewDefaultWorkerSessionManager[application.Session]()
	concurrentSessionManager := wrappers.NewConcurrentManager(sessionManager)

	th := udp_chacha20.NewTunHandler(
		ctx,
		chacha20.NewUdpReader(tun),
		network.NewIPV4HeaderParser(),
		concurrentSessionManager,
	)

	sock, err := s.socketFactory.newSocket(s.settings.ConnectionIP, s.settings.Port)
	if err != nil {
		return nil, err
	}
	ul := s.udpFactory.listenUDP(sock)

	conf, confErr := s.configurationManager.Configuration()
	if confErr != nil {
		return nil, confErr
	}

	tr := udp_chacha20.NewTransportHandler(
		ctx,
		s.settings,
		tun,
		ul,
		concurrentSessionManager,
		s.loggerFactory.newLogger(),
		NewHandshakeFactory(*conf),
		chacha20.NewUdpSessionBuilder(),
	)
	return udp_chacha20.NewUdpTunWorker(th, tr), nil
}
