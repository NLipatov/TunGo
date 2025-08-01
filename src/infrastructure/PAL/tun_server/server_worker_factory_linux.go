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
	"tungo/infrastructure/network"
	"tungo/infrastructure/routing/server_routing/routing/tcp_chacha20"
	"tungo/infrastructure/routing/server_routing/routing/udp_chacha20"
	"tungo/infrastructure/routing/server_routing/session_management/repository"
	"tungo/infrastructure/routing/server_routing/session_management/repository/wrappers"
	"tungo/infrastructure/settings"
)

type ServerWorkerFactory struct {
	settings             settings.Settings
	loggerFactory        loggerFactory
	configurationManager server.ServerConfigurationManager
}

func NewServerWorkerFactory(
	settings settings.Settings,
	manager server.ServerConfigurationManager,
) application.ServerWorkerFactory {
	return &ServerWorkerFactory{
		settings:             settings,
		loggerFactory:        newDefaultLoggerFactory(),
		configurationManager: manager,
	}
}

func NewTestServerWorkerFactory(
	settings settings.Settings,
	loggerFactory loggerFactory,
	manager server.ServerConfigurationManager,
) application.ServerWorkerFactory {
	return &ServerWorkerFactory{
		settings:             settings,
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

	conf, confErr := s.configurationManager.Configuration()
	if confErr != nil {
		return nil, confErr
	}

	addrPort, addrPortErr := s.addrPortToListen(s.settings.ConnectionIP, s.settings.Port)
	if addrPortErr != nil {
		return nil, addrPortErr
	}

	listener, err := net.Listen("tcp", addrPort.String())
	if err != nil {
		return nil, fmt.Errorf("failed to listen TCP: %w", err)
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

	conf, confErr := s.configurationManager.Configuration()
	if confErr != nil {
		return nil, confErr
	}

	addrPort, addrPortErr := s.addrPortToListen(s.settings.ConnectionIP, s.settings.Port)
	if addrPortErr != nil {
		return nil, addrPortErr
	}

	conn, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(addrPort))
	if err != nil {
		return nil, fmt.Errorf("failed to listen on port: %s", err)
	}

	tr := udp_chacha20.NewTransportHandler(
		ctx,
		s.settings,
		tun,
		conn,
		concurrentSessionManager,
		s.loggerFactory.newLogger(),
		NewHandshakeFactory(*conf),
		chacha20.NewUdpSessionBuilder(),
	)
	return udp_chacha20.NewUdpTunWorker(th, tr), nil
}

func (s *ServerWorkerFactory) addrPortToListen(ip, port string) (netip.AddrPort, error) {
	if ip == "" {
		ip = "::" // dual-stack listen - both ipv4 and ipv6
	}
	return netip.ParseAddrPort(net.JoinHostPort(ip, port))
}
