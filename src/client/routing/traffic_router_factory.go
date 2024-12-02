package routing

import (
	"fmt"
	"log"
	"tungo/client/routing/tuntcp"
	"tungo/client/routing/tunudp"
	"tungo/client/tunconf"
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
	tunConfiguratorFactory := tunconf.NewTunConfiguratorFactory()
	tunConfigurator, tunConfiguratorFactoryErr := tunConfiguratorFactory.CreateTunConfigurator()
	if tunConfiguratorFactoryErr != nil {
		log.Fatalf("failed to create a %v tun configurator: %s", conf.Protocol, tunConfiguratorFactoryErr)
	}

	tunConfigurator.Deconfigure(conf.TCPSettings)
	tunConfigurator.Deconfigure(conf.UDPSettings)

	switch conf.Protocol {
	case settings.TCP:
		return &tuntcp.TCPRouter{
			Settings:        conf.TCPSettings,
			TunConfigurator: tunConfigurator,
		}, nil
	case settings.UDP:
		return &tunudp.UDPRouter{
			Settings:        conf.UDPSettings,
			TunConfigurator: tunConfigurator,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported conf: %v", conf)
	}
}
