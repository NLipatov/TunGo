package routing

import (
	"fmt"
	"tungo/infrastructure/tun/linux_tun"
	"tungo/presentation/client_routing/routing/tcp_chacha20"
	"tungo/presentation/client_routing/routing/udp_chacha20"
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
	linuxTunConfigurator := linux_tun.LinuxTunConfigurator{}
	linuxTunConfigurator.Dispose(conf.TCPSettings)
	linuxTunConfigurator.Dispose(conf.UDPSettings)

	switch conf.Protocol {
	case settings.TCP:
		configuredTunDevice, configuredTunDeviceErr := linuxTunConfigurator.Configure(conf.TCPSettings)
		if configuredTunDeviceErr != nil {
			return nil, configuredTunDeviceErr
		}

		return &tcp_chacha20.TCPRouter{
			Settings: conf.TCPSettings,
			Tun:      linux_tun.NewDisposableTunDevice(configuredTunDevice, conf.TCPSettings),
		}, nil
	case settings.UDP:
		configuredTunDevice, configuredTunDeviceErr := linuxTunConfigurator.Configure(conf.UDPSettings)
		if configuredTunDeviceErr != nil {
			return nil, configuredTunDeviceErr
		}

		return &udp_chacha20.UDPRouter{
			Settings: conf.UDPSettings,
			Tun:      linux_tun.NewDisposableTunDevice(configuredTunDevice, conf.UDPSettings),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported conf: %v", conf)
	}
}
