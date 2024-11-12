package routing

import (
	"context"
	"fmt"
	"tungo/client/forwarding/clienttunconf"
	"tungo/client/forwarding/routing/routers"
	"tungo/settings"
	"tungo/settings/client"
)

type ClientRouter struct {
}

func (cr ClientRouter) Route(conf client.Conf, ctx context.Context) error {
	// Clear existing client configuration
	clienttunconf.Deconfigure(conf.TCPSettings)
	clienttunconf.Deconfigure(conf.UDPSettings)

	switch conf.Protocol {
	case settings.TCP:
		return routers.StartTCPRouting(conf.TCPSettings, ctx)
	case settings.UDP:
		return routers.StartUDPRouting(conf.UDPSettings, ctx)

	default:
		return fmt.Errorf("invalid protocol: %v", conf.Protocol)
	}
}
