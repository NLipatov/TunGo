package factory

import (
	"fmt"
	"io"
	"net"
	"tungo/application"
	"tungo/presentation/client_routing/routing/tcp_chacha20"
	"tungo/presentation/client_routing/routing/udp_chacha20"
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
	conn net.Conn, tun io.ReadWriteCloser, cryptographyService application.CryptographyService,
) (application.TunWorker, error) {
	switch w.conf.Protocol {
	case settings.UDP:
		return udp_chacha20.NewUdpWorker(conn, tun, cryptographyService), nil
	case settings.TCP:
		return tcp_chacha20.NewTcpTunWorker(conn, tun, cryptographyService), nil
	default:
		return nil, fmt.Errorf("unsupported protocol")
	}
}
