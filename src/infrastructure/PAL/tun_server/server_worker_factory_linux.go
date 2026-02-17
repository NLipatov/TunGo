package tun_server

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
	"tungo/infrastructure/cryptography/noise"
	"tungo/infrastructure/network/ip"
	wsServer "tungo/infrastructure/network/ws/server/factory"
	"tungo/infrastructure/settings"
	"tungo/infrastructure/tunnel/dataplane/server/tcp_chacha20"
	"tungo/infrastructure/tunnel/dataplane/server/udp_chacha20"
	"tungo/infrastructure/tunnel/session"
	"tungo/infrastructure/tunnel/sessionplane/server/tcp_registration"
	"tungo/infrastructure/tunnel/sessionplane/server/udp_registration"
)

type ServerWorkerFactory struct {
	loggerFactory        loggerFactory
	configurationManager server.ConfigurationManager
	sessionRevoker       *session.CompositeSessionRevoker
	allowedPeers         noise.AllowedPeersLookup
	cookieManager        *noise.CookieManager
	loadMonitor          *noise.LoadMonitor
}

func NewServerWorkerFactory(
	manager server.ConfigurationManager,
) (*ServerWorkerFactory, error) {
	conf, err := manager.Configuration()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	cookieManager, err := noise.NewCookieManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie manager: %w", err)
	}

	return &ServerWorkerFactory{
		loggerFactory:        newDefaultLoggerFactory(),
		configurationManager: manager,
		sessionRevoker:       session.NewCompositeSessionRevoker(),
		allowedPeers:         noise.NewAllowedPeersLookup(conf.AllowedPeers),
		cookieManager:        cookieManager,
		loadMonitor:          noise.NewLoadMonitor(noise.DefaultLoadThreshold),
	}, nil
}

func NewTestServerWorkerFactory(
	loggerFactory loggerFactory,
	manager server.ConfigurationManager,
) (*ServerWorkerFactory, error) {
	conf, err := manager.Configuration()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	cookieManager, err := noise.NewCookieManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie manager: %w", err)
	}

	return &ServerWorkerFactory{
		loggerFactory:        loggerFactory,
		configurationManager: manager,
		sessionRevoker:       session.NewCompositeSessionRevoker(),
		allowedPeers:         noise.NewAllowedPeersLookup(conf.AllowedPeers),
		cookieManager:        cookieManager,
		loadMonitor:          noise.NewLoadMonitor(noise.DefaultLoadThreshold),
	}, nil
}

// SessionRevoker returns the composite session revoker that aggregates
// all session repositories created by this factory.
// Used by ConfigWatcher to revoke sessions when AllowedPeers changes.
func (s *ServerWorkerFactory) SessionRevoker() *session.CompositeSessionRevoker {
	return s.sessionRevoker
}

// AllowedPeersUpdater returns the AllowedPeers lookup for runtime updates.
// Used by ConfigWatcher to update peer map when config changes.
func (s *ServerWorkerFactory) AllowedPeersUpdater() server.AllowedPeersUpdater {
	return s.allowedPeers
}

func (s *ServerWorkerFactory) CreateWorker(
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

func (s *ServerWorkerFactory) createTCPWorker(
	ctx context.Context,
	tun io.ReadWriteCloser,
	workerSettings settings.Settings,
) (routing.Worker, error) {
	sessionManager := session.NewDefaultRepository()
	// Register for session revocation on config changes
	if revocable, ok := sessionManager.(session.RepositoryWithRevocation); ok {
		s.sessionRevoker.Register(revocable)
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

	handshakeFactory := NewHandshakeFactory(*conf, s.allowedPeers, s.cookieManager, s.loadMonitor)

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

func (s *ServerWorkerFactory) createWSWorker(
	ctx context.Context,
	tun io.ReadWriteCloser,
	workerSettings settings.Settings,
) (routing.Worker, error) {
	sessionManager := session.NewDefaultRepository()
	// Register for session revocation on config changes
	if revocable, ok := sessionManager.(session.RepositoryWithRevocation); ok {
		s.sessionRevoker.Register(revocable)
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

	handshakeFactory := NewHandshakeFactory(*conf, s.allowedPeers, s.cookieManager, s.loadMonitor)

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

func (s *ServerWorkerFactory) createUDPWorker(
	ctx context.Context,
	tun io.ReadWriteCloser,
	workerSettings settings.Settings,
) (routing.Worker, error) {
	sessionManager := session.NewDefaultRepository()
	// Register for session revocation on config changes
	if revocable, ok := sessionManager.(session.RepositoryWithRevocation); ok {
		s.sessionRevoker.Register(revocable)
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

	handshakeFactory := NewHandshakeFactory(*conf, s.allowedPeers, s.cookieManager, s.loadMonitor)

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
		logger,
		registrar,
	)
	return udp_chacha20.NewUdpTunWorker(th, tr), nil
}

func (s *ServerWorkerFactory) addrPortToListen(
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
