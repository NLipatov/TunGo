package routing

import (
	"fmt"
	"tungo/client/forwarding/routing/tuntcp"
	"tungo/client/forwarding/routing/tunudp"
	"tungo/settings"
	"tungo/settings/client"
)

// RouterFactory is responsible for creating Router instances.
type RouterFactory struct{}

// NewRouterFactory creates a new instance of RouterFactory.
func NewRouterFactory() *RouterFactory {
	return &RouterFactory{}
}

// CreateRouter creates a Router instance for the specified protocol.
func (f *RouterFactory) CreateRouter(conf client.Conf) (Router, error) {
	switch conf.Protocol {
	case settings.TCP:
		return &tuntcp.TCPRouter{
			Settings: conf.TCPSettings,
		}, nil
	case settings.UDP:
		return &tunudp.UDPRouter{
			Settings: conf.UDPSettings,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported conf: %v", conf)
	}
}
