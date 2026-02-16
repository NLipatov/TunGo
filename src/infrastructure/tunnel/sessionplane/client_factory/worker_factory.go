package client_factory

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"time"
	"tungo/application/network/connection"
	"tungo/application/network/routing"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/network/udp/adapters"
	"tungo/infrastructure/settings"
	"tungo/infrastructure/tunnel/dataplane/client/tcp_chacha20"
	"tungo/infrastructure/tunnel/dataplane/client/udp_chacha20"
)

type WorkerFactory struct {
	conf client.Configuration
}

func NewWorkerFactory(configuration client.Configuration) connection.ClientWorkerFactory {
	return &WorkerFactory{
		conf: configuration,
	}
}

func (w *WorkerFactory) CreateWorker(
	ctx context.Context, conn connection.Transport, tun io.ReadWriteCloser, crypto connection.Crypto, controller *rekey.StateMachine,
) (routing.Worker, error) {
	allowed := w.allowedSources()

	switch w.conf.Protocol {
	case settings.UDP:
		udpConn, err := unwrapUDPConn(conn)
		if err != nil {
			return nil, err
		}
		deadline := time.Second
		transport := adapters.NewClientUDPAdapter(udpConn, deadline, deadline)
		egress := connection.NewDefaultEgress(transport, crypto)
		// tunHandler reads from tun and writes to transport
		tunHandler := udp_chacha20.NewTunHandler(
			ctx,
			tun,
			egress,
			controller,
			allowed,
		)
		// transportHandler reads from transport and writes to tun
		transportHandler := udp_chacha20.NewTransportHandler(
			ctx,
			transport,
			tun,
			crypto,
			controller,
			egress,
		)
		return udp_chacha20.NewUdpWorker(transportHandler, tunHandler), nil
	case settings.TCP, settings.WS, settings.WSS:
		egress := connection.NewDefaultEgress(conn, crypto)
		tunHandler := tcp_chacha20.NewTunHandler(ctx, tun, egress, controller, allowed)
		transportHandler := tcp_chacha20.NewTransportHandler(ctx, conn, tun, crypto, controller, egress)
		return tcp_chacha20.NewTcpTunWorker(ctx, tunHandler, transportHandler, crypto, controller), nil
	default:
		return nil, fmt.Errorf("unsupported protocol")
	}
}

func unwrapUDPConn(transport connection.Transport) (*net.UDPConn, error) {
	current := transport
	for i := 0; i < 8; i++ {
		if udpConn, ok := current.(*net.UDPConn); ok {
			return udpConn, nil
		}
		unwrapper, ok := current.(interface{ Unwrap() connection.Transport })
		if !ok {
			break
		}
		next := unwrapper.Unwrap()
		if next == nil || next == current {
			break
		}
		current = next
	}
	return nil, fmt.Errorf("udp transport must wrap *net.UDPConn, got %T", transport)
}

func (w *WorkerFactory) allowedSources() map[netip.Addr]struct{} {
	s, err := w.conf.ActiveSettings()
	if err != nil {
		return nil
	}
	m := make(map[netip.Addr]struct{}, 2)
	if s.IPv4.IsValid() {
		m[s.IPv4.Unmap()] = struct{}{}
	}
	if s.IPv6.IsValid() {
		m[s.IPv6.Unmap()] = struct{}{}
	}
	return m
}
