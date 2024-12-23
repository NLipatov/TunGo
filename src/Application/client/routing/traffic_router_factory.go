package routing

import (
	"fmt"
	"tungo/Application/client/routing/tun_tcp_chacha20"
	"tungo/Application/client/routing/tun_udp_chacha20"
	"tungo/Application/client/tun_configurator"
	"tungo/Domain/settings"
	"tungo/Domain/settings/client"
)

// RouterFactory is responsible for creating TrafficRouter instances.
type RouterFactory struct{}

// NewRouterFactory creates a new instance of RouterFactory.
func NewRouterFactory() *RouterFactory {
	return &RouterFactory{}
}

// CreateRouter creates a TrafficRouter instance for the specified protocol.
func (f *RouterFactory) CreateRouter(conf client.Conf) (TrafficRouter, error) {
	switch conf.Protocol {
	case settings.TCP:
		return &tun_tcp_chacha20.TCPRouter{
			Settings:        conf.TCPSettings,
			TunConfigurator: &tun_configurator.LinuxTunConfigurator{},
		}, nil
	case settings.UDP:
		return &tun_udp_chacha20.UDPRouter{
			Settings:        conf.UDPSettings,
			TunConfigurator: &tun_configurator.LinuxTunConfigurator{},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported conf: %v", conf)
	}
}
