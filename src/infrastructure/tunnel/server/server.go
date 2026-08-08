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
	"tungo/infrastructure/cryptography/noise"
	"tungo/infrastructure/network/ws"
	"tungo/infrastructure/telemetry/trafficstats"
	"tungo/infrastructure/tunnel/server/internal/session"
	tcpserver "tungo/infrastructure/tunnel/server/internal/tcp"
	udpserver "tungo/infrastructure/tunnel/server/internal/udp"

	"golang.org/x/sync/errgroup"
)

// Run starts every enabled protocol and blocks until one fails or ctx is
// cancelled. ready is called after all listeners and TUN devices are ready.
func (s *Server) Run(ctx context.Context, ready func()) error {
	if err := s.cleanup(); err != nil {
		slog.Warn("preflight cleanup error", "err", err)
	}
	defer func() {
		if err := s.cleanup(); err != nil {
			slog.Warn("postflight cleanup error", "err", err)
		}
	}()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	group, groupCtx := errgroup.WithContext(runCtx)
	for _, profile := range s.configuration.Profiles() {
		if !profile.Enabled {
			continue
		}
		run, device, err := s.createTunnel(groupCtx, profile.Settings)
		if err != nil {
			cancel()
			_ = group.Wait()
			return fmt.Errorf("could not create %s tunnel: %w", profile.Settings.Protocol, err)
		}
		protocol := profile.Settings.Protocol
		group.Go(func() error {
			defer func() { _ = device.Close() }()
			if err := run(); err != nil {
				return fmt.Errorf("%s tunnel failed: %w", protocol, err)
			}
			return nil
		})
	}
	ready()
	return group.Wait()
}

func (s *Server) cleanup() error {
	var group errgroup.Group
	for _, profile := range s.configuration.Profiles() {
		group.Go(func() error { return s.tunManager.DisposeDevices(profile.Settings) })
	}
	return group.Wait()
}

func (s *Server) createTunnel(
	ctx context.Context,
	workerSettings settings.Settings,
) (func() error, io.ReadWriteCloser, error) {
	device, err := s.tunManager.CreateDevice(workerSettings)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating tun device: %w", err)
	}
	run, err := s.newTunnel(ctx, device, workerSettings)
	if err != nil {
		_ = device.Close()
		return nil, nil, fmt.Errorf("error creating tunnel: %w", err)
	}
	return run, device, nil
}

func (s *Server) newTunnel(
	ctx context.Context,
	tun io.ReadWriteCloser,
	workerSettings settings.Settings,
) (func() error, error) {
	tun = trafficstats.WrapTun(tun)
	switch workerSettings.Protocol {
	case settings.TCP:
		return s.newTCPTunnel(ctx, tun, workerSettings)
	case settings.UDP:
		return s.newUDPTunnel(ctx, tun, workerSettings)
	case settings.WS, settings.WSS:
		return s.newWSTunnel(ctx, tun, workerSettings)
	default:
		return nil, fmt.Errorf("protocol %v not supported", workerSettings.Protocol)
	}
}

var _ appConfiguration.ServerSessionRevoker = (*Server)(nil)
var _ appConfiguration.ServerAllowedPeersUpdater = (*Server)(nil)

func (s *Server) newHandshake() *noise.IKHandshake {
	return noise.NewIKHandshakeServer(
		s.configuration.X25519PublicKey,
		s.configuration.X25519PrivateKey,
		s.allowedPeers,
		s.cookieManager,
		s.loadMonitor,
	)
}

func (s *Server) newTCPTunnel(
	ctx context.Context,
	tun io.ReadWriteCloser,
	workerSettings settings.Settings,
) (func() error, error) {
	sessionManager := session.NewRepository()

	addrPort, addrPortErr := s.addrPortToListen(workerSettings.Server, workerSettings.Port)
	if addrPortErr != nil {
		return nil, addrPortErr
	}

	listener, err := net.Listen("tcp", addrPort.String())
	if err != nil {
		return nil, fmt.Errorf("failed to listen TCP: %w", err)
	}
	slog.Info("server listening", "protocol", workerSettings.Protocol, "address", listener.Addr())

	s.register(sessionManager)

	server := tcpserver.New(
		ctx, tun, listener, sessionManager, s.newHandshake,
		workerSettings.IPv4Subnet, workerSettings.IPv6Subnet,
	)
	return server.Run, nil
}

func (s *Server) newWSTunnel(
	ctx context.Context,
	tun io.ReadWriteCloser,
	workerSettings settings.Settings,
) (func() error, error) {
	sessionManager := session.NewRepository()

	addrPort, addrPortErr := s.addrPortToListen(workerSettings.Server, workerSettings.Port)
	if addrPortErr != nil {
		return nil, addrPortErr
	}

	tcpListener, tcpListenerErr := net.Listen("tcp", addrPort.String())
	if tcpListenerErr != nil {
		return nil, fmt.Errorf("failed to listen TCP: %w", tcpListenerErr)
	}

	wsListener, wsListenerErr := ws.NewListener(ctx, tcpListener)
	if wsListenerErr != nil {
		_ = tcpListener.Close()
		return nil, fmt.Errorf("failed to listen WebSocket: %w", wsListenerErr)
	}
	slog.Info("server listening", "protocol", workerSettings.Protocol, "address", tcpListener.Addr())

	s.register(sessionManager)

	server := tcpserver.New(
		ctx, tun, wsListener, sessionManager, s.newHandshake,
		workerSettings.IPv4Subnet, workerSettings.IPv6Subnet,
	)
	return server.Run, nil
}

func (s *Server) newUDPTunnel(
	ctx context.Context,
	tun io.ReadWriteCloser,
	workerSettings settings.Settings,
) (func() error, error) {
	sessionManager := session.NewRepository()

	addrPort, addrPortErr := s.addrPortToListen(workerSettings.Server, workerSettings.Port)
	if addrPortErr != nil {
		return nil, addrPortErr
	}

	conn, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(addrPort))
	if err != nil {
		return nil, fmt.Errorf("failed to listen on port: %s", err)
	}
	slog.Info("server listening", "protocol", workerSettings.Protocol, "address", conn.LocalAddr())

	s.register(sessionManager)

	server := udpserver.New(
		ctx, tun, conn, sessionManager, s.newHandshake,
		workerSettings.IPv4Subnet, workerSettings.IPv6Subnet,
	)
	return server.Run, nil
}

func (s *Server) addrPortToListen(
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
