package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"tungo/application/network/routing"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/network/ip"
	wsServer "tungo/infrastructure/network/ws/server/factory"
	"tungo/infrastructure/settings"
	"tungo/infrastructure/tunnel/dataplane/server/tcp_chacha20"
	"tungo/infrastructure/tunnel/dataplane/server/udp_chacha20"
	"tungo/infrastructure/tunnel/session"
	"tungo/infrastructure/tunnel/sessionplane/server/tcp_registration"
	"tungo/infrastructure/tunnel/sessionplane/server/udp_registration"
)

type WorkerFactory struct {
	loggerFactory        loggerFactory
	configurationManager server.ConfigurationManager
	runtime              *Runtime
}

func NewWorkerFactory(
	runtime *Runtime,
	manager server.ConfigurationManager,
) (*WorkerFactory, error) {
	return &WorkerFactory{
		loggerFactory:        newDefaultLoggerFactory(),
		configurationManager: manager,
		runtime:              runtime,
	}, nil
}

func NewTestWorkerFactory(
	loggerFactory loggerFactory,
	runtime *Runtime,
	manager server.ConfigurationManager,
) (*WorkerFactory, error) {
	return &WorkerFactory{
		loggerFactory:        loggerFactory,
		configurationManager: manager,
		runtime:              runtime,
	}, nil
}

func (s *WorkerFactory) CreateWorker(
	ctx context.Context,
	tun io.ReadWriteCloser,
	workerSettings settings.Settings,
) (routing.Worker, error) {
	switch workerSettings.Protocol {
	case settings.TCP:
		return s.createTCPWorker(ctx, tun, workerSettings)
	case settings.UDP:
		return s.createUDPWorker(ctx, tun, workerSettings)
	case settings.WS, settings.WSS:
		return s.createWSWorker(ctx, tun, workerSettings)
	default:
		return nil, fmt.Errorf("protocol %v not supported", workerSettings.Protocol)
	}
}

func (s *WorkerFactory) createTCPWorker(
	ctx context.Context,
	tun io.ReadWriteCloser,
	workerSettings settings.Settings,
) (routing.Worker, error) {
	sessionManager := session.NewDefaultRepository()
	if revocable, ok := sessionManager.(session.RepositoryWithRevocation); ok {
		s.runtime.sessionRevoker.Register(revocable)
	}

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

	addrPort, addrPortErr := s.addrPortToListen(workerSettings.Server, workerSettings.Port)
	if addrPortErr != nil {
		return nil, addrPortErr
	}

	listener, err := net.Listen("tcp", addrPort.String())
	if err != nil {
		return nil, fmt.Errorf("failed to listen TCP: %w", err)
	}

	logger := s.loggerFactory.newLogger()

	handshakeFactory := NewHandshakeFactory(*conf, s.runtime.allowedPeers, s.runtime.cookieManager, s.runtime.loadMonitor)

	registrar := tcp_registration.NewRegistrar(
		logger,
		handshakeFactory,
		chacha20.NewTcpSessionBuilder(chacha20.NewDefaultAEADBuilder()),
		sessionManager,
		workerSettings.IPv4Subnet,
		workerSettings.IPv6Subnet,
	)

	tr := tcp_chacha20.NewTransportHandler(
		ctx,
		workerSettings,
		tun,
		listener,
		sessionManager,
		logger,
		registrar,
	)
	return tcp_chacha20.NewTcpTunWorker(th, tr), nil
}

func (s *WorkerFactory) createWSWorker(
	ctx context.Context,
	tun io.ReadWriteCloser,
	workerSettings settings.Settings,
) (routing.Worker, error) {
	sessionManager := session.NewDefaultRepository()
	if revocable, ok := sessionManager.(session.RepositoryWithRevocation); ok {
		s.runtime.sessionRevoker.Register(revocable)
	}

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

	addrPort, addrPortErr := s.addrPortToListen(workerSettings.Server, workerSettings.Port)
	if addrPortErr != nil {
		return nil, addrPortErr
	}

	tcpListener, tcpListenerErr := net.Listen("tcp", addrPort.String())
	if tcpListenerErr != nil {
		return nil, fmt.Errorf("failed to listen TCP: %w", tcpListenerErr)
	}

	wsListenerFactory := wsServer.NewDefaultListenerFactory()
	wsListener, wsListenerErr := wsListenerFactory.NewListener(ctx, tcpListener)
	if wsListenerErr != nil {
		_ = tcpListener.Close()
		return nil, fmt.Errorf("failed to listen WebSocket: %w", wsListenerErr)
	}

	logger := s.loggerFactory.newLogger()

	handshakeFactory := NewHandshakeFactory(*conf, s.runtime.allowedPeers, s.runtime.cookieManager, s.runtime.loadMonitor)

	registrar := tcp_registration.NewRegistrar(
		logger,
		handshakeFactory,
		chacha20.NewTcpSessionBuilder(chacha20.NewDefaultAEADBuilder()),
		sessionManager,
		workerSettings.IPv4Subnet,
		workerSettings.IPv6Subnet,
	)

	tr := tcp_chacha20.NewTransportHandler(
		ctx,
		workerSettings,
		tun,
		wsListener,
		sessionManager,
		logger,
		registrar,
	)
	return tcp_chacha20.NewTcpTunWorker(th, tr), nil
}

func (s *WorkerFactory) createUDPWorker(
	ctx context.Context,
	tun io.ReadWriteCloser,
	workerSettings settings.Settings,
) (routing.Worker, error) {
	sessionManager := session.NewDefaultRepository()
	if revocable, ok := sessionManager.(session.RepositoryWithRevocation); ok {
		s.runtime.sessionRevoker.Register(revocable)
	}

	conf, confErr := s.configurationManager.Configuration()
	if confErr != nil {
		return nil, confErr
	}

	addrPort, addrPortErr := s.addrPortToListen(workerSettings.Server, workerSettings.Port)
	if addrPortErr != nil {
		return nil, addrPortErr
	}

	conn, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(addrPort))
	if err != nil {
		return nil, fmt.Errorf("failed to listen on port: %s", err)
	}

	logger := s.loggerFactory.newLogger()

	th := udp_chacha20.NewTunHandler(
		ctx,
		tun,
		ip.NewHeaderParser(),
		sessionManager,
	)

	handshakeFactory := NewHandshakeFactory(*conf, s.runtime.allowedPeers, s.runtime.cookieManager, s.runtime.loadMonitor)

	registrar := udp_registration.NewRegistrar(
		ctx,
		conn,
		sessionManager,
		logger,
		handshakeFactory,
		chacha20.NewUdpSessionBuilder(chacha20.NewDefaultAEADBuilder()),
		workerSettings.IPv4Subnet,
		workerSettings.IPv6Subnet,
	)

	tr := udp_chacha20.NewTransportHandler(
		ctx,
		workerSettings,
		tun,
		conn,
		sessionManager,
		sessionManager,
		logger,
		registrar,
	)
	return udp_chacha20.NewUdpTunWorker(th, tr), nil
}

func (s *WorkerFactory) addrPortToListen(
	host settings.Host,
	port int,
) (netip.AddrPort, error) {
	return host.ListenAddrPort(port, listenFallbackIP())
}

// listenFallbackIP returns "::" on dual-stack/IPv6-only systems, or "0.0.0.0"
// when the kernel has IPv6 disabled (e.g. ipv6.disable=1).
var listenFallbackIP = sync.OnceValue(func() string {
	ln, err := net.Listen("tcp", "[::]:0")
	if err != nil {
		return "0.0.0.0"
	}
	_ = ln.Close()
	return "::"
})
