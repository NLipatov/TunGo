package client_factory

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"time"
	appConfiguration "tungo/application/configuration"
	"tungo/application/configuration/settings"
	"tungo/application/network/connection"
	"tungo/application/network/routing"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/udp/adapters"
	"tungo/infrastructure/tunnel/controlplane"
	"tungo/infrastructure/tunnel/dataplane/client/tcp_chacha20"
	"tungo/infrastructure/tunnel/dataplane/client/udp_chacha20"
)

type WorkerFactory struct {
	conf appConfiguration.ClientRuntimeConfiguration
}

func NewWorkerFactory(configuration appConfiguration.ClientRuntimeConfiguration) connection.ClientWorkerFactory {
	return &WorkerFactory{
		conf: configuration,
	}
}

func (w *WorkerFactory) CreateWorker(
	ctx context.Context, conn connection.Transport, tun io.ReadWriteCloser, crypto connection.Crypto, controller connection.RekeyController,
) (routing.Endpoints, error) {
	allowed := w.allowedSources()
	rekeyCoordinator := controlplane.NewClientRekeyCoordinator(
		&primitives.DefaultKeyDeriver{},
		controller,
		settings.DefaultRekeyInterval,
		time.Now().UTC(),
	)

	switch w.conf.Settings.Protocol {
	case settings.UDP:
		udpConn, err := unwrapUDPConn(conn)
		if err != nil {
			return routing.Endpoints{}, err
		}
		deadline := time.Second
		transport := adapters.NewClientUDPAdapter(udpConn, deadline, deadline)
		egress := connection.NewEgress(transport, crypto)
		// tunHandler reads from tun and writes to transport
		tunHandler := udp_chacha20.NewTunHandler(
			ctx,
			tun,
			egress,
			rekeyCoordinator,
			allowed,
		)
		// transportHandler reads from transport and writes to tun
		transportHandler := udp_chacha20.NewTransportHandler(
			ctx,
			transport,
			tun,
			crypto,
			controller,
			rekeyCoordinator,
			egress,
		)
		return routing.Endpoints{RunTun: tunHandler.HandleTun, RunTransport: transportHandler.HandleTransport}, nil
	case settings.TCP, settings.WS, settings.WSS:
		egress := connection.NewEgress(conn, crypto)
		tunHandler := tcp_chacha20.NewTunHandler(ctx, tun, egress, rekeyCoordinator, allowed)
		transportHandler := tcp_chacha20.NewTransportHandler(ctx, conn, tun, crypto, controller, rekeyCoordinator, egress)
		return routing.Endpoints{RunTun: tunHandler.HandleTun, RunTransport: transportHandler.HandleTransport}, nil
	default:
		return routing.Endpoints{}, fmt.Errorf("unsupported protocol")
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
	s := w.conf.Settings
	m := make(map[netip.Addr]struct{}, 2)
	if s.IPv4.IsValid() {
		m[s.IPv4.Unmap()] = struct{}{}
	}
	if s.IPv6.IsValid() {
		m[s.IPv6.Unmap()] = struct{}{}
	}
	return m
}
