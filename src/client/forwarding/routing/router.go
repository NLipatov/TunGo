package routing

import (
	"context"
	"etha-tunnel/client/forwarding/ipconfiguration"
	"etha-tunnel/client/forwarding/routing/routerImpl"
	"etha-tunnel/network"
	"etha-tunnel/settings"
	"fmt"
	"log"
	"os"
)

// CRouter client-side router
type CRouter interface {
	StartRouting(settings settings.ConnectionSettings, tunFile *os.File, ctx *context.Context) error
}

type ClientRouter struct {
}

func (cr ClientRouter) Route(connSettings settings.ConnectionSettings, ctx context.Context) error {
	switch connSettings.Protocol {
	case settings.TCP:
		// Configure client
		if tcpConfigurationErr := ipconfiguration.Configure(connSettings); tcpConfigurationErr != nil {
			log.Fatalf("Failed to configure client: %v", tcpConfigurationErr)
		}
		defer ipconfiguration.Unconfigure(connSettings)

		// Open the TUN interface
		tunFile, openTunErr := network.OpenTunByName(connSettings.InterfaceName)
		if openTunErr != nil {
			log.Fatalf("Failed to open TUN interface: %v", openTunErr)
		}
		defer tunFile.Close()

		return routerImpl.StartTCPRouting(connSettings, tunFile, ctx)
	case settings.UDP:
		// Configure client
		if udpConfigurationErr := ipconfiguration.Configure(connSettings); udpConfigurationErr != nil {
			log.Fatalf("Failed to configure client: %v", udpConfigurationErr)
		}
		defer ipconfiguration.Unconfigure(connSettings)

		// Open the TUN interface
		tunFile, openTunErr := network.OpenTunByName(connSettings.InterfaceName)
		if openTunErr != nil {
			log.Fatalf("Failed to open TUN interface: %v", openTunErr)
		}
		defer tunFile.Close()

		routingErr := routerImpl.StartUDPRouting(connSettings, tunFile, ctx)
		if routingErr != nil {
			log.Fatalf("failed to route trafic: %s", routingErr)
		}
		return routerImpl.StartUDPRouting(connSettings, tunFile, ctx)
	default:
		return fmt.Errorf("invalid protocol: %v", connSettings.Protocol)
	}
}
