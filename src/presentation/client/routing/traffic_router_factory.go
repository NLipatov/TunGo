package routing

import (
	"fmt"
	"log"
	"tungo/presentation/client/routing/tcp_chacha20"
	"tungo/presentation/client/routing/udp_chacha20"
	"tungo/presentation/client/tun_configurator"
	"tungo/settings"
	"tungo/settings/client"
)

// RouterFactory is responsible for creating TrafficRouter instances.
type RouterFactory struct{}

// NewRouterFactory creates a new instance of RouterFactory.
func NewRouterFactory() *RouterFactory {
	return &RouterFactory{}
}

// CreateRouter creates a TrafficRouter instance for the specified protocol.
func (f *RouterFactory) CreateRouter(conf client.Conf) (TrafficRouter, error) {
	tunConfiguratorFactory := tun_configurator.NewTunConfiguratorFactory()
	tunConfigurator, tunConfiguratorFactoryErr := tunConfiguratorFactory.CreateTunConfigurator()
	if tunConfiguratorFactoryErr != nil {
		log.Fatalf("failed to create a %v tun configurator: %s", conf.Protocol, tunConfiguratorFactoryErr)
	}

	tunConfigurator.Deconfigure(conf.TCPSettings)
	tunConfigurator.Deconfigure(conf.UDPSettings)

	switch conf.Protocol {
	case settings.TCP:
		return &tcp_chacha20.TCPRouter{
			Settings:        conf.TCPSettings,
			TunConfigurator: tunConfigurator,
		}, nil
	case settings.UDP:
		return &udp_chacha20.UDPRouter{
			Settings:        conf.UDPSettings,
			TunConfigurator: tunConfigurator,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported conf: %v", conf)
	}
}
