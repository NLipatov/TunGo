package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	appConfiguration "tungo/application/configuration"
	"tungo/application/network/routing"
	"tungo/infrastructure/cryptography/chacha20/tcp"
	"tungo/infrastructure/cryptography/chacha20/udp"
	wsServer "tungo/infrastructure/network/ws/server/factory"
	"tungo/infrastructure/settings"
	"tungo/infrastructure/telemetry/trafficstats"
	"tungo/infrastructure/tunnel/dataplane/server/tcp_chacha20"
	"tungo/infrastructure/tunnel/dataplane/server/udp_chacha20"
	"tungo/infrastructure/tunnel/session"
	"tungo/infrastructure/tunnel/sessionplane/server/tcp_registration"
	"tungo/infrastructure/tunnel/sessionplane/server/udp_registration"
)

type WorkerFactory struct {
	configuration appConfiguration.ServerRuntimeConfiguration
	runtime       *Runtime
}

func NewWorkerFactory(
	runtime *Runtime,
	configuration appConfiguration.ServerRuntimeConfiguration,
) (*WorkerFactory, error) {
	return &WorkerFactory{
		configuration: configuration,
		runtime:       runtime,
	}, nil
}

func (s *WorkerFactory) CreateWorker(
	ctx context.Context,
	tun io.ReadWriteCloser,
	workerSettings settings.Settings,
) (routing.Endpoints, error) {
	tun = trafficstats.WrapTun(tun)
	switch workerSettings.Protocol {
	case settings.TCP:
		return s.createTCPWorker(ctx, tun, workerSettings)
	case settings.UDP:
		return s.createUDPWorker(ctx, tun, workerSettings)
	case settings.WS, settings.WSS:
		return s.createWSWorker(ctx, tun, workerSettings)
	default:
		return routing.Endpoints{}, fmt.Errorf("protocol %v not supported", workerSettings.Protocol)
	}
}

func (s *WorkerFactory) createTCPWorker(
	ctx context.Context,
	tun io.ReadWriteCloser,
	workerSettings settings.Settings,
) (routing.Endpoints, error) {
	sessionManager := session.NewDefaultRepository()
	s.runtime.sessionRevoker.Register(sessionManager)

	addrPort, addrPortErr := s.addrPortToListen(workerSettings.Server, workerSettings.Port)
	if addrPortErr != nil {
		return routing.Endpoints{}, addrPortErr
	}

	listener, err := net.Listen("tcp", addrPort.String())
	if err != nil {
		return routing.Endpoints{}, fmt.Errorf("failed to listen TCP: %w", err)
	}

	handshakeFactory := NewHandshakeFactory(s.configuration, s.runtime.allowedPeers, s.runtime.cookieManager, s.runtime.loadMonitor)

	registrar := tcp_registration.NewRegistrar(
		handshakeFactory,
		tcp.NewFactory(),
		sessionManager,
		workerSettings.IPv4Subnet,
		workerSettings.IPv6Subnet,
	)

	server := tcp_chacha20.NewServer(ctx, workerSettings, tun, listener, sessionManager, registrar)
	return routing.Endpoints{RunTun: server.RunTun, RunTransport: server.RunTransport}, nil
}

func (s *WorkerFactory) createWSWorker(
	ctx context.Context,
	tun io.ReadWriteCloser,
	workerSettings settings.Settings,
) (routing.Endpoints, error) {
	sessionManager := session.NewDefaultRepository()
	s.runtime.sessionRevoker.Register(sessionManager)

	addrPort, addrPortErr := s.addrPortToListen(workerSettings.Server, workerSettings.Port)
	if addrPortErr != nil {
		return routing.Endpoints{}, addrPortErr
	}

	tcpListener, tcpListenerErr := net.Listen("tcp", addrPort.String())
	if tcpListenerErr != nil {
		return routing.Endpoints{}, fmt.Errorf("failed to listen TCP: %w", tcpListenerErr)
	}

	wsListenerFactory := wsServer.NewDefaultListenerFactory()
	wsListener, wsListenerErr := wsListenerFactory.NewListener(ctx, tcpListener)
	if wsListenerErr != nil {
		_ = tcpListener.Close()
		return routing.Endpoints{}, fmt.Errorf("failed to listen WebSocket: %w", wsListenerErr)
	}

	handshakeFactory := NewHandshakeFactory(s.configuration, s.runtime.allowedPeers, s.runtime.cookieManager, s.runtime.loadMonitor)

	registrar := tcp_registration.NewRegistrar(
		handshakeFactory,
		tcp.NewFactory(),
		sessionManager,
		workerSettings.IPv4Subnet,
		workerSettings.IPv6Subnet,
	)

	server := tcp_chacha20.NewServer(ctx, workerSettings, tun, wsListener, sessionManager, registrar)
	return routing.Endpoints{RunTun: server.RunTun, RunTransport: server.RunTransport}, nil
}

func (s *WorkerFactory) createUDPWorker(
	ctx context.Context,
	tun io.ReadWriteCloser,
	workerSettings settings.Settings,
) (routing.Endpoints, error) {
	sessionManager := session.NewDefaultRepository()
	s.runtime.sessionRevoker.Register(sessionManager)

	addrPort, addrPortErr := s.addrPortToListen(workerSettings.Server, workerSettings.Port)
	if addrPortErr != nil {
		return routing.Endpoints{}, addrPortErr
	}

	conn, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(addrPort))
	if err != nil {
		return routing.Endpoints{}, fmt.Errorf("failed to listen on port: %s", err)
	}

	handshakeFactory := NewHandshakeFactory(s.configuration, s.runtime.allowedPeers, s.runtime.cookieManager, s.runtime.loadMonitor)

	registrar := udp_registration.NewRegistrar(
		ctx,
		conn,
		sessionManager,
		handshakeFactory,
		udp.NewFactory(),
		workerSettings.IPv4Subnet,
		workerSettings.IPv6Subnet,
	)

	server := udp_chacha20.NewServer(ctx, workerSettings, tun, conn, sessionManager, registrar)
	return routing.Endpoints{RunTun: server.RunTun, RunTransport: server.RunTransport}, nil
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
