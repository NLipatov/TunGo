package factory

import (
	"context"
	"fmt"
	"io"
	"net"
	"tungo/application"
	"tungo/routing_layer/client_routing/routing/tcp_chacha20"
	"tungo/routing_layer/client_routing/routing/udp_chacha20"
	"tungo/settings"
	"tungo/settings/client_configuration"
)

type WorkerFactory struct {
	conf client_configuration.Configuration
}

func NewWorkerFactory(configuration client_configuration.Configuration) application.TunWorkerFactory {
	return &WorkerFactory{
		conf: configuration,
	}
}

func (w *WorkerFactory) CreateWorker(
	ctx context.Context, conn net.Conn, tun io.ReadWriteCloser, cryptographyService application.CryptographyService,
) (application.TunWorker, error) {
	switch w.conf.Protocol {
	case settings.UDP:
		return udp_chacha20.NewUdpWorker(ctx, conn, tun, cryptographyService), nil
	case settings.TCP:
		return tcp_chacha20.NewTcpTunWorker(ctx, conn, tun, cryptographyService), nil
	default:
		return nil, fmt.Errorf("unsupported protocol")
	}
}
