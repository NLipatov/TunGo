package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"sync"

	"tungo/internal/config"
	"tungo/internal/config/settings"
	"tungo/internal/protocol/noise"
	"tungo/internal/server/session"
	tcpserver "tungo/internal/server/tcp"
	udpserver "tungo/internal/server/udp"
	"tungo/internal/trafficstats"
	"tungo/internal/transport/ws"
	servertun "tungo/internal/tun/server"

	"golang.org/x/sync/errgroup"
)

type protocolTunnel interface {
	Run() error
}

// New builds a server that owns all configured protocol tunnels.
func New() (*Server, error) {
	control := config.NewServerControl()
	if control == nil {
		return nil, fmt.Errorf("server runtime is not supported on this platform")
	}
	conf, err := control.ServerConfiguration()
	if err != nil {
		return nil, fmt.Errorf("failed to load server configuration: %w", err)
	}

	cookieManager, err := noise.NewCookieManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie manager: %w", err)
	}

	return &Server{
		configuration: conf,
		tunManager:    servertun.NewManager(),
		control:       control,
		allowedPeers:  newAllowedPeers(conf.AllowedPeers),
		cookieManager: cookieManager,
		loadMonitor:   noise.NewLoadMonitor(noise.DefaultLoadThreshold),
	}, nil
}

// Run starts every enabled protocol and watches authorization changes until
// the server stops.
func (s *Server) Run(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	watcherDone := make(chan struct{})
	if s.control != nil {
		go func() {
			defer close(watcherDone)
			s.control.WatchServerConfiguration(runCtx, s, s)
		}()
	} else {
		close(watcherDone)
	}

	err := s.run(runCtx)
	cancel()
	<-watcherDone
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func (s *Server) Ready() bool {
	return s.ready.Load()
}

func (s *Server) run(ctx context.Context) error {
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
		tunnel, device, err := s.createTunnel(groupCtx, profile.Settings)
		if err != nil {
			cancel()
			_ = group.Wait()
			return fmt.Errorf("could not create %s tunnel: %w", profile.Settings.Protocol, err)
		}
		protocol := profile.Settings.Protocol
		group.Go(func() error {
			defer func() { _ = device.Close() }()
			if err := tunnel.Run(); err != nil {
				return fmt.Errorf("%s tunnel failed: %w", protocol, err)
			}
			return nil
		})
	}
	s.ready.Store(true)
	return group.Wait()
}

func (s *Server) cleanup() error {
	var group errgroup.Group
	for _, profile := range s.configuration.Profiles() {
		group.Go(func() error { return s.tunManager.CloseTunnel(profile.Settings) })
	}
	return group.Wait()
}

func (s *Server) createTunnel(
	ctx context.Context,
	workerSettings settings.Settings,
) (protocolTunnel, io.ReadWriteCloser, error) {
	device, err := s.tunManager.OpenTunnel(workerSettings)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating tun device: %w", err)
	}
	tunnel, err := s.newTunnel(ctx, device, workerSettings)
	if err != nil {
		_ = device.Close()
		return nil, nil, fmt.Errorf("error creating tunnel: %w", err)
	}
	return tunnel, device, nil
}

func (s *Server) newTunnel(
	ctx context.Context,
	tun io.ReadWriter,
	workerSettings settings.Settings,
) (protocolTunnel, error) {
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

var _ config.ServerSessionRevoker = (*Server)(nil)
var _ config.ServerAllowedPeersUpdater = (*Server)(nil)

func (s *Server) newTCPTunnel(
	ctx context.Context,
	tun io.ReadWriter,
	workerSettings settings.Settings,
) (protocolTunnel, error) {
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
		ctx, tun, listener, sessionManager,
		func() *noise.IKHandshake {
			return noise.NewIKHandshakeServer(
				s.configuration.X25519PublicKey,
				s.configuration.X25519PrivateKey,
				s.allowedPeers,
				s.cookieManager,
				s.loadMonitor,
			)
		},
		workerSettings.IPv4Subnet, workerSettings.IPv6Subnet,
	)
	return server, nil
}

func (s *Server) newWSTunnel(
	ctx context.Context,
	tun io.ReadWriter,
	workerSettings settings.Settings,
) (protocolTunnel, error) {
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
		ctx, tun, wsListener, sessionManager,
		func() *noise.IKHandshake {
			return noise.NewIKHandshakeServer(
				s.configuration.X25519PublicKey,
				s.configuration.X25519PrivateKey,
				s.allowedPeers,
				s.cookieManager,
				s.loadMonitor,
			)
		},
		workerSettings.IPv4Subnet, workerSettings.IPv6Subnet,
	)
	return server, nil
}

func (s *Server) newUDPTunnel(
	ctx context.Context,
	tun io.ReadWriter,
	workerSettings settings.Settings,
) (protocolTunnel, error) {
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
		ctx, tun, conn, sessionManager,
		func() *noise.IKHandshake {
			return noise.NewIKHandshakeServer(
				s.configuration.X25519PublicKey,
				s.configuration.X25519PrivateKey,
				s.allowedPeers,
				s.cookieManager,
				s.loadMonitor,
			)
		},
		workerSettings.IPv4Subnet, workerSettings.IPv6Subnet,
	)
	return server, nil
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
