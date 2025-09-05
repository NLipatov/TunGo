package tun_server

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"tungo/application"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/network/ip"
	"tungo/infrastructure/network/service"
	wsServer "tungo/infrastructure/network/ws/server"
	"tungo/infrastructure/routing/server_routing/routing/tcp_chacha20"
	"tungo/infrastructure/routing/server_routing/routing/udp_chacha20"
	"tungo/infrastructure/routing/server_routing/session_management/repository"
	"tungo/infrastructure/routing/server_routing/session_management/repository/wrappers"
	"tungo/infrastructure/settings"
)

type ServerWorkerFactory struct {
	loggerFactory        loggerFactory
	configurationManager server.ServerConfigurationManager
}

func NewServerWorkerFactory(
	manager server.ServerConfigurationManager,
) application.ServerWorkerFactory {
	return &ServerWorkerFactory{
		loggerFactory:        newDefaultLoggerFactory(),
		configurationManager: manager,
	}
}

func NewTestServerWorkerFactory(
	loggerFactory loggerFactory,
	manager server.ServerConfigurationManager,
) application.ServerWorkerFactory {
	return &ServerWorkerFactory{
		loggerFactory:        loggerFactory,
		configurationManager: manager,
	}
}

func (s *ServerWorkerFactory) CreateWorker(
	ctx context.Context,
	tun io.ReadWriteCloser,
	workerSettings settings.Settings,
) (application.TunWorker, error) {
	switch workerSettings.Protocol {
	case settings.TCP:
		return s.createTCPWorker(ctx, tun, workerSettings)
	case settings.UDP:
		return s.createUDPWorker(ctx, tun, workerSettings)
	case settings.WS:
		return s.createWSWorker(ctx, tun, workerSettings)
	default:
		return nil, fmt.Errorf("protocol %v not supported", workerSettings.Protocol)
	}
}

func (s *ServerWorkerFactory) createTCPWorker(
	ctx context.Context,
	tun io.ReadWriteCloser,
	workerSettings settings.Settings,
) (application.TunWorker, error) {
	sessionManager := wrappers.NewConcurrentManager(
		repository.NewDefaultWorkerSessionManager[application.Session](),
	)

	th := tcp_chacha20.NewTunHandler(
		ctx,
		tun,
		ip.NewHeaderParser(),
		sessionManager,
	)

	conf, confErr := s.configurationManager.Configuration()
	if confErr != nil {
		return nil, confErr
	}

	addrPort, addrPortErr := s.addrPortToListen(workerSettings.ConnectionIP, workerSettings.Port)
	if addrPortErr != nil {
		return nil, addrPortErr
	}

	listener, err := net.Listen("tcp", addrPort.String())
	if err != nil {
		return nil, fmt.Errorf("failed to listen TCP: %w", err)
	}

	tr := tcp_chacha20.NewTransportHandler(
		ctx,
		workerSettings,
		tun,
		listener,
		sessionManager,
		s.loggerFactory.newLogger(),
		NewHandshakeFactory(*conf),
		chacha20.NewTcpSessionBuilder(chacha20.NewDefaultAEADBuilder()),
	)
	return tcp_chacha20.NewTcpTunWorker(th, tr), nil
}

func (s *ServerWorkerFactory) createWSWorker(
	ctx context.Context,
	tun io.ReadWriteCloser,
	workerSettings settings.Settings,
) (application.TunWorker, error) {
	sessionManager := wrappers.NewConcurrentManager(
		repository.NewDefaultWorkerSessionManager[application.Session](),
	)

	th := tcp_chacha20.NewTunHandler(
		ctx,
		tun,
		ip.NewHeaderParser(),
		sessionManager,
	)

	conf, confErr := s.configurationManager.Configuration()
	if confErr != nil {
		return nil, confErr
	}

	addrPort, addrPortErr := s.addrPortToListen(workerSettings.ConnectionIP, workerSettings.Port)
	if addrPortErr != nil {
		return nil, addrPortErr
	}

	tcpListener, tcpListenerErr := net.Listen("tcp", addrPort.String())
	if tcpListenerErr != nil {
		return nil, fmt.Errorf("failed to listen TCP: %w", tcpListenerErr)
	}

	wsListener, wsListenerErr := wsServer.NewDefaultListener(ctx, tcpListener)
	if wsListenerErr != nil {
		return nil, fmt.Errorf("failed to listen WebSocket: %w", wsListenerErr)
	}

	tr := tcp_chacha20.NewTransportHandler(
		ctx,
		workerSettings,
		tun,
		wsListener,
		sessionManager,
		s.loggerFactory.newLogger(),
		NewHandshakeFactory(*conf),
		chacha20.NewTcpSessionBuilder(chacha20.NewDefaultAEADBuilder()),
	)
	return tcp_chacha20.NewTcpTunWorker(th, tr), nil
}

func (s *ServerWorkerFactory) createUDPWorker(
	ctx context.Context,
	tun io.ReadWriteCloser,
	workerSettings settings.Settings,
) (application.TunWorker, error) {
	sessionManager := wrappers.NewConcurrentManager(
		repository.NewDefaultWorkerSessionManager[application.Session](),
	)

	th := udp_chacha20.NewTunHandler(
		ctx,
		service.NewDefaultAdapter(tun),
		ip.NewHeaderParser(),
		sessionManager,
	)

	conf, confErr := s.configurationManager.Configuration()
	if confErr != nil {
		return nil, confErr
	}

	addrPort, addrPortErr := s.addrPortToListen(workerSettings.ConnectionIP, workerSettings.Port)
	if addrPortErr != nil {
		return nil, addrPortErr
	}

	conn, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(addrPort))
	if err != nil {
		return nil, fmt.Errorf("failed to listen on port: %s", err)
	}

	tr := udp_chacha20.NewTransportHandler(
		ctx,
		workerSettings,
		tun,
		conn,
		sessionManager,
		s.loggerFactory.newLogger(),
		NewHandshakeFactory(*conf),
		chacha20.NewUdpSessionBuilder(chacha20.NewDefaultAEADBuilder()),
	)
	return udp_chacha20.NewUdpTunWorker(th, tr), nil
}

func (s *ServerWorkerFactory) addrPortToListen(
	ip, port string,
) (netip.AddrPort, error) {
	if ip == "" {
		ip = "::" // dual-stack listen - both ipv4 and ipv6
	}
	return netip.ParseAddrPort(net.JoinHostPort(ip, port))
}
