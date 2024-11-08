package routing

import (
	"context"
	"fmt"
	"log"
	"os"
	"tungo/client/forwarding/clientipconf"
	"tungo/network/ip"
	"tungo/settings"
	"tungo/settings/client"
)

type ClientRouter struct {
}

func (cr ClientRouter) Route(conf client.Conf, ctx context.Context) error {
	// Clear existing client configuration
	clientipconf.Unconfigure(conf.TCPSettings)
	clientipconf.Unconfigure(conf.UDPSettings)

	switch conf.Protocol {
	case settings.TCP:
		defer clientipconf.Unconfigure(conf.TCPSettings)
		tunFile := configureTun(conf.TCPSettings)
		defer func() {
			_ = tunFile.Close()
		}()
		return startTCPRouting(conf.TCPSettings, tunFile, ctx)

	case settings.UDP:
		defer clientipconf.Unconfigure(conf.UDPSettings)
		tunFile := configureTun(conf.UDPSettings)
		defer func() {
			_ = tunFile.Close()
		}()
		return startUDPRouting(conf.UDPSettings, tunFile, ctx)

	default:
		return fmt.Errorf("invalid protocol: %v", conf.Protocol)
	}
}

func configureTun(s settings.ConnectionSettings) *os.File {
	// Configure client
	if udpConfigurationErr := clientipconf.Configure(s); udpConfigurationErr != nil {
		log.Fatalf("failed to configure client: %v", udpConfigurationErr)
	}

	if setMtuErr := ip.SetMtu(s.InterfaceName, s.MTU); setMtuErr != nil {
		log.Fatalf("failed to set %d MTU for %s: %s", s.MTU, s.InterfaceName, setMtuErr)
	}

	// Open the TUN interface
	tunFile, openTunErr := ip.OpenTunByName(s.InterfaceName)
	if openTunErr != nil {
		log.Fatalf("failed to open TUN interface: %v", openTunErr)
	}

	return tunFile
}
