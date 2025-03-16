package routing

import (
	"fmt"
	"tungo/infrastructure/tun_device"
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
	tunDevice, tunDeviceErr := tun_device.NewTunDevice(conf)
	if tunDeviceErr != nil {
		return nil, tunDeviceErr
	}

	switch conf.Protocol {
	case settings.TCP:
		return &tcp_chacha20.TCPRouter{
			Settings: conf.TCPSettings,
			Tun:      tunDevice,
		}, nil
	case settings.UDP:
		return &udp_chacha20.UDPRouter{
			Settings: conf.UDPSettings,
			Tun:      tunDevice,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported conf: %v", conf)
	}
}
