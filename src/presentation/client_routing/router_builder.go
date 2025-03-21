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
	tun, tunErr := tunDevConfigurator.CreateTunDevice()
	if tunErr != nil {
		log.Printf("failed to create tun: %s", tunErr)
	}

	switch conf.Protocol {
	case settings.UDP:
		conn, cryptographyService, connErr := connectionFactory.EstablishConnection(ctx, conf.UDPSettings)
		if connErr != nil {
			return nil, connErr
		}
		return udp_chacha20.NewUDPRouter(conn.(*net.UDPConn), tun, cryptographyService), nil
	case settings.TCP:
		conn, cryptographyService, connErr := connectionFactory.EstablishConnection(ctx, conf.TCPSettings)
		if connErr != nil {
			return nil, connErr
		}
		return tcp_chacha20.NewTCPRouter(conn, tun, cryptographyService), nil
	default:
		return nil, fmt.Errorf("unsupported protocol")
	}
}
