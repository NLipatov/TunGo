package client_factory

import (
	"context"
	"fmt"
	"io"
	"net"
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
	switch w.conf.Protocol {
	case settings.UDP:
		deadline := time.Second
		transport := adapters.NewClientUDPAdapter(conn.(*net.UDPConn), deadline, deadline)
		egress := connection.NewDefaultEgress(transport, crypto)
		// tunHandler reads from tun and writes to transport
		tunHandler := udp_chacha20.NewTunHandler(
			ctx,
			tun,
			egress,
			controller,
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
	case settings.TCP:
		tunHandler := tcp_chacha20.NewTunHandler(ctx, tun, conn, crypto, controller)
		transportHandler := tcp_chacha20.NewTransportHandler(ctx, conn, tun, crypto, controller)
		return tcp_chacha20.NewTcpTunWorker(ctx, tunHandler, transportHandler, crypto, controller), nil
	case settings.WS:
		tunHandler := tcp_chacha20.NewTunHandler(ctx, tun, conn, crypto, controller)
		transportHandler := tcp_chacha20.NewTransportHandler(ctx, conn, tun, crypto, controller)
		return tcp_chacha20.NewTcpTunWorker(ctx, tunHandler, transportHandler, crypto, controller), nil
	default:
		return nil, fmt.Errorf("unsupported protocol")
	}
}
