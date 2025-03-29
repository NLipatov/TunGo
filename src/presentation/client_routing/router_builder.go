package client_routing

import (
	"context"
	"fmt"
	"log"
	"net"
	"tungo/application"
	"tungo/presentation/client_routing/routing/tcp_chacha20"
	"tungo/presentation/client_routing/routing/udp_chacha20"
	"tungo/settings"
	"tungo/settings/client_configuration"
)

type RouterBuilder struct {
}

func NewRouterBuilder() RouterBuilder {
	return RouterBuilder{}
}

func (u *RouterBuilder) Build(
	ctx context.Context, conf client_configuration.Configuration, tunDevConfigurator application.PlatformTunConfigurator,
) (application.TrafficRouter, error) {
	connectionFactory := NewConnectionFactory()

	switch conf.Protocol {
	case settings.UDP:
		conn, cryptographyService, connErr := connectionFactory.EstablishConnection(ctx, conf.UDPSettings)
		if connErr != nil {
			return nil, connErr
		}

		tun, tunErr := tunDevConfigurator.CreateTunDevice()
		if tunErr != nil {
			log.Printf("failed to create tun: %s", tunErr)
			return nil, tunErr
		}

		worker := udp_chacha20.NewUdpWorker(*conn.(*net.UDPConn), tun, cryptographyService)

		return NewRouter(worker), nil
	case settings.TCP:
		conn, cryptographyService, connErr := connectionFactory.EstablishConnection(ctx, conf.TCPSettings)
		if connErr != nil {
			return nil, connErr
		}

		tun, tunErr := tunDevConfigurator.CreateTunDevice()
		if tunErr != nil {
			log.Printf("failed to create tun: %s", tunErr)
			return nil, tunErr
		}

		worker := tcp_chacha20.NewTcpTunWorker(conn, tun, cryptographyService)

		return NewRouter(worker), nil
	default:
		return nil, fmt.Errorf("unsupported protocol")
	}
}
