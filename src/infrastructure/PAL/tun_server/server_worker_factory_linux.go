package tun_server

import (
	"context"
	"fmt"
	"io"
	"net"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/listeners/udp_listener"
	"tungo/infrastructure/logging"
	"tungo/infrastructure/network"
	"tungo/infrastructure/routing/server_routing/routing/tcp_chacha20"
	"tungo/infrastructure/routing/server_routing/routing/udp_chacha20"
	"tungo/infrastructure/routing/server_routing/session_management"
	"tungo/infrastructure/settings"
)

type ServerWorkerFactory struct {
	settings settings.Settings
}

func NewServerWorkerFactory(settings settings.Settings) application.ServerWorkerFactory {
	return &ServerWorkerFactory{
		settings: settings,
	}
}

func (s ServerWorkerFactory) CreateWorker(ctx context.Context, tun io.ReadWriteCloser) (application.TunWorker, error) {
	switch s.settings.Protocol {
	case settings.TCP:
		return s.createTCPWorker(ctx, tun)
	case settings.UDP:
		return s.createUDPWorker(ctx, tun)
	default:
		return nil, fmt.Errorf("protocol %v not supported", s.settings.Protocol)
	}
}

func (s ServerWorkerFactory) createTCPWorker(ctx context.Context, tun io.ReadWriteCloser) (application.TunWorker, error) {
	smFactory := newSessionManagerFactory[tcp_chacha20.Session]()
	concurrentSessionManager := smFactory.createConcurrentManager()
	tunHandler := s.buildTCPTunHandler(ctx, tun, concurrentSessionManager)
	socket, socketErr := s.createSocket()
	if socketErr != nil {
		return nil, socketErr
	}
	listener, err := s.createTCPListener(socket)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on port %s: %v", s.settings.Port, err)
	}
	transportHandler := s.buildTCPTransportHandler(ctx, tun, concurrentSessionManager, listener)
	return tcp_chacha20.NewTcpTunWorker(tunHandler, transportHandler), nil
}

func (s ServerWorkerFactory) createUDPWorker(ctx context.Context, tun io.ReadWriteCloser) (application.TunWorker, error) {
	smFactory := newSessionManagerFactory[udp_chacha20.Session]()
	concurrentSessionManager := smFactory.createConcurrentManager()
	tunHandler := s.buildUDPTunHandler(ctx, tun, concurrentSessionManager)
	socket, socketErr := s.createSocket()
	if socketErr != nil {
		return nil, socketErr
	}
	listener := s.createUDPListener(socket)
	transportHandler := s.buildUDPTransportHandler(ctx, tun, concurrentSessionManager, listener)
	return udp_chacha20.NewUdpTunWorker(tunHandler, transportHandler), nil
}

func (s ServerWorkerFactory) newLogger() application.Logger {
	return logging.NewLogLogger()
}

func (s ServerWorkerFactory) buildTCPTunHandler(
	ctx context.Context,
	tun io.ReadWriteCloser,
	mgr session_management.WorkerSessionManager[tcp_chacha20.Session],
) application.TunHandler {
	tunHandler := tcp_chacha20.NewTunHandler(
		ctx,
		chacha20.NewTcpReader(tun),
		chacha20.NewDefaultTCPEncoder(),
		network.NewIPV4HeaderParser(),
		mgr)

	return tunHandler
}
func (s ServerWorkerFactory) buildUDPTunHandler(
	ctx context.Context,
	tun io.ReadWriteCloser,
	mgr session_management.WorkerSessionManager[udp_chacha20.Session],
) application.TunHandler {
	tunHandler := udp_chacha20.NewTunHandler(ctx,
		chacha20.NewUdpReader(tun),
		network.NewIPV4HeaderParser(),
		mgr)

	return tunHandler
}

func (s ServerWorkerFactory) buildTCPTransportHandler(
	ctx context.Context,
	tun io.ReadWriteCloser,
	mgr session_management.WorkerSessionManager[tcp_chacha20.Session],
	listener net.Listener) application.TransportHandler {
	return tcp_chacha20.NewTransportHandler(
		ctx,
		s.settings,
		tun,
		listener,
		mgr,
		s.newLogger())
}

func (s ServerWorkerFactory) buildUDPTransportHandler(
	ctx context.Context,
	tun io.ReadWriteCloser,
	mgr session_management.WorkerSessionManager[udp_chacha20.Session],
	listener udp_listener.Listener) application.TransportHandler {
	return udp_chacha20.NewTransportHandler(
		ctx,
		s.settings,
		tun,
		listener,
		mgr,
		s.newLogger())
}

func (s ServerWorkerFactory) createTCPListener(socket application.Socket) (net.Listener, error) {
	return net.Listen("tcp", socket.StringAddr())
}

func (s ServerWorkerFactory) createUDPListener(socket application.Socket) udp_listener.Listener {
	return udp_listener.NewUdpListener(socket)
}

func (s ServerWorkerFactory) createSocket() (application.Socket, error) {
	return network.NewSocket(s.settings.ConnectionIP, s.settings.Port)
}
