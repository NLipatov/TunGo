package client_factory

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"
	"tungo/application/network/connection"
	"tungo/application/network/routing"
	"tungo/domain/network/service"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/network"
	"tungo/infrastructure/network/udp/adapters"
	"tungo/infrastructure/routing/client_routing/routing/tcp_chacha20"
	"tungo/infrastructure/routing/client_routing/routing/udp_chacha20"
	"tungo/infrastructure/settings"
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
	ctx context.Context, conn connection.Transport, tun io.ReadWriteCloser, crypto connection.Crypto,
) (routing.Worker, error) {
	switch w.conf.Protocol {
	case settings.UDP:
		deadline, deadlineErr := network.NewDeadline(time.Second * 1)
		if deadlineErr != nil {
			return nil, deadlineErr
		}
		mtu := settings.ResolveMTU(w.conf.UDPSettings.MTU)
		transport := adapters.NewClientUDPAdapter(conn.(*net.UDPConn), deadline, deadline, mtu)
		// tunHandler reads from tun and writes to transport
		tunHandler := udp_chacha20.NewTunHandler(ctx, tun, transport, crypto, mtu)
		// transportHandler reads from transport and writes to tun
		transportHandler := udp_chacha20.NewTransportHandler(
			ctx,
			transport,
			tun,
			crypto,
			service.NewDefaultPacketHandler(),
			mtu,
		)
		return udp_chacha20.NewUdpWorker(transportHandler, tunHandler), nil
	case settings.TCP:
		tunHandler := tcp_chacha20.NewTunHandler(ctx, tun, conn, crypto)
		transportHandler := tcp_chacha20.NewTransportHandler(ctx, conn, tun, crypto)
		return tcp_chacha20.NewTcpTunWorker(ctx, tunHandler, transportHandler, crypto), nil
	case settings.WS:
		tunHandler := tcp_chacha20.NewTunHandler(ctx, tun, conn, crypto)
		transportHandler := tcp_chacha20.NewTransportHandler(ctx, conn, tun, crypto)
		return tcp_chacha20.NewTcpTunWorker(ctx, tunHandler, transportHandler, crypto), nil
	default:
		return nil, fmt.Errorf("unsupported protocol")
	}
}
