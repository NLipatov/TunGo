package routing

import (
	"context"
	"etha-tunnel/client/forwarding/ipconfiguration"
	"etha-tunnel/network"
	"etha-tunnel/settings"
	"etha-tunnel/settings/client"
	"fmt"
	"log"
	"os"
)

type ClientRouter struct {
}

func (cr ClientRouter) Route(conf client.Conf, ctx context.Context) error {
	// Clear existing client configuration
	ipconfiguration.Unconfigure(conf.TCPSettings)
	ipconfiguration.Unconfigure(conf.UDPSettings)

	switch conf.Protocol {
	case settings.TCP:
		defer ipconfiguration.Unconfigure(conf.TCPSettings)
		tunFile := configureTun(conf.TCPSettings)
		defer func() {
			_ = tunFile.Close()
		}()
		return startTCPRouting(conf.TCPSettings, tunFile, ctx)

	case settings.UDP:
		defer ipconfiguration.Unconfigure(conf.UDPSettings)
		tunFile := configureTun(conf.UDPSettings)
		defer func() {
			_ = tunFile.Close()
		}()
		return startUDPRouting(conf.UDPSettings, tunFile, ctx)

	default:
		return fmt.Errorf("invalid protocol: %v", conf.Protocol)
	}
}

func configureTun(connectionSettings settings.ConnectionSettings) *os.File {
	// Configure client
	if udpConfigurationErr := ipconfiguration.Configure(connectionSettings); udpConfigurationErr != nil {
		log.Fatalf("Failed to configure client: %v", udpConfigurationErr)
	}

	// Open the TUN interface
	tunFile, openTunErr := network.OpenTunByName(connectionSettings.InterfaceName)
	if openTunErr != nil {
		log.Fatalf("Failed to open TUN interface: %v", openTunErr)
	}

	return tunFile
}
