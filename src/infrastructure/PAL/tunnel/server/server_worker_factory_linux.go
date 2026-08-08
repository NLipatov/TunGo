package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"sync"
	appConfiguration "tungo/application/configuration"
	"tungo/application/configuration/settings"
	"tungo/application/network/routing"
	"tungo/infrastructure/cryptography/chacha20/tcp"
	"tungo/infrastructure/cryptography/chacha20/udp"
	"tungo/infrastructure/network/ws"
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
	sessionManager := session.NewRepository()
	s.runtime.sessionRevoker.Register(sessionManager)

	addrPort, addrPortErr := s.addrPortToListen(workerSettings.Server, workerSettings.Port)
	if addrPortErr != nil {
		return routing.Endpoints{}, addrPortErr
	}

	listener, err := net.Listen("tcp", addrPort.String())
	if err != nil {
		return routing.Endpoints{}, fmt.Errorf("failed to listen TCP: %w", err)
	}
	slog.Info("server listening", "protocol", workerSettings.Protocol, "address", listener.Addr())

	handshakeFactory := NewHandshakeFactory(s.configuration, s.runtime.allowedPeers, s.runtime.cookieManager, s.runtime.loadMonitor)

	registrar := tcp_registration.NewRegistrar(
		handshakeFactory,
		tcp.NewFactory(),
		sessionManager,
		workerSettings.IPv4Subnet,
		workerSettings.IPv6Subnet,
	)

	server := tcp_chacha20.NewServer(ctx, tun, listener, sessionManager, registrar)
	return routing.Endpoints{RunTun: server.RunTun, RunTransport: server.RunTransport}, nil
}

func (s *WorkerFactory) createWSWorker(
	ctx context.Context,
	tun io.ReadWriteCloser,
	workerSettings settings.Settings,
) (routing.Endpoints, error) {
	sessionManager := session.NewRepository()
	s.runtime.sessionRevoker.Register(sessionManager)

	addrPort, addrPortErr := s.addrPortToListen(workerSettings.Server, workerSettings.Port)
	if addrPortErr != nil {
		return routing.Endpoints{}, addrPortErr
	}

	tcpListener, tcpListenerErr := net.Listen("tcp", addrPort.String())
	if tcpListenerErr != nil {
		return routing.Endpoints{}, fmt.Errorf("failed to listen TCP: %w", tcpListenerErr)
	}

	wsListener, wsListenerErr := ws.NewListener(ctx, tcpListener)
	if wsListenerErr != nil {
		_ = tcpListener.Close()
		return routing.Endpoints{}, fmt.Errorf("failed to listen WebSocket: %w", wsListenerErr)
	}
	slog.Info("server listening", "protocol", workerSettings.Protocol, "address", tcpListener.Addr())

	handshakeFactory := NewHandshakeFactory(s.configuration, s.runtime.allowedPeers, s.runtime.cookieManager, s.runtime.loadMonitor)

	registrar := tcp_registration.NewRegistrar(
		handshakeFactory,
		tcp.NewFactory(),
		sessionManager,
		workerSettings.IPv4Subnet,
		workerSettings.IPv6Subnet,
	)

	server := tcp_chacha20.NewServer(ctx, tun, wsListener, sessionManager, registrar)
	return routing.Endpoints{RunTun: server.RunTun, RunTransport: server.RunTransport}, nil
}

func (s *WorkerFactory) createUDPWorker(
	ctx context.Context,
	tun io.ReadWriteCloser,
	workerSettings settings.Settings,
) (routing.Endpoints, error) {
	sessionManager := session.NewRepository()
	s.runtime.sessionRevoker.Register(sessionManager)

	addrPort, addrPortErr := s.addrPortToListen(workerSettings.Server, workerSettings.Port)
	if addrPortErr != nil {
		return routing.Endpoints{}, addrPortErr
	}

	conn, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(addrPort))
	if err != nil {
		return routing.Endpoints{}, fmt.Errorf("failed to listen on port: %s", err)
	}
	slog.Info("server listening", "protocol", workerSettings.Protocol, "address", conn.LocalAddr())

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

	server := udp_chacha20.NewServer(ctx, tun, conn, sessionManager, registrar)
	return routing.Endpoints{RunTun: server.RunTun, RunTransport: server.RunTransport}, nil
}

func (s *WorkerFactory) addrPortToListen(
	host settings.Host,
	port int,
) (netip.AddrPort, error) {
	if port < 1 || port > 65535 {
		return netip.AddrPort{}, fmt.Errorf("invalid port: %d", port)
	}

	rawIP := host.IPv4
	if rawIP == "" {
		rawIP = host.IPv6
	}
	if rawIP == "" {
		if host.Domain != "" {
			return netip.AddrPort{}, fmt.Errorf("host %q is not an IP address", host.Domain)
		}
		rawIP = listenFallbackIP()
	}
	ip, err := netip.ParseAddr(rawIP)
	if err != nil {
		return netip.AddrPort{}, err
	}
	return netip.AddrPortFrom(ip.Unmap(), uint16(port)), nil
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
