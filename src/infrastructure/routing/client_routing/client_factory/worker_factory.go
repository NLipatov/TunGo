package client_factory

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"
	"tungo/application"
	"tungo/infrastructure/PAL/client_configuration"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/network"
	"tungo/infrastructure/routing/client_routing/routing/tcp_chacha20"
	"tungo/infrastructure/routing/client_routing/routing/udp_chacha20"
	"tungo/infrastructure/settings"
)

type WorkerFactory struct {
	conf client_configuration.Configuration
}

func NewWorkerFactory(configuration client_configuration.Configuration) application.ClientWorkerFactory {
	return &WorkerFactory{
		conf: configuration,
	}
}

func (w *WorkerFactory) CreateWorker(
	ctx context.Context, conn application.ConnectionAdapter, tun io.ReadWriteCloser, crypto application.CryptographyService,
) (application.TunWorker, error) {
	switch w.conf.Protocol {
	case settings.UDP:
		deadline, deadlineErr := network.NewDeadline(time.Second * 1)
		if deadlineErr != nil {
			return nil, deadlineErr
		}
		transport := network.NewClientUDPAdapter(
			conn.(*net.UDPConn),
			deadline,
			deadline,
		)
		// tunHandler reads from tun and writes to transport
		tunHandler := udp_chacha20.NewTunHandler(ctx, chacha20.NewUdpReader(tun), transport, crypto)
		// transportHandler reads from transport and writes to tun
		transportHandler := udp_chacha20.NewTransportHandler(ctx, transport, tun, crypto)
		return udp_chacha20.NewUdpWorker(transportHandler, tunHandler), nil
	case settings.TCP:
		transport := chacha20.NewTCPFramingAdapter(
			conn,
		)
		tunHandler := tcp_chacha20.NewTunHandler(ctx, tun, transport, crypto)
		transportHandler := tcp_chacha20.NewTransportHandler(ctx, transport, tun, crypto)
		return tcp_chacha20.NewTcpTunWorker(ctx, tunHandler, transportHandler, crypto), nil
	default:
		return nil, fmt.Errorf("unsupported protocol")
	}
}
