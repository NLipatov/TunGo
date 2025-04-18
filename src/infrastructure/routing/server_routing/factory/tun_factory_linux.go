package factory

import (
	"fmt"
	"log"
	"tungo/application"
	"tungo/infrastructure/platform_tun/tools_linux"
	"tungo/infrastructure/platform_tun/tools_linux/ip"
	"tungo/settings"
)

type ServerTunFactory struct {
}

func NewServerTunFactory() application.ServerTunManager {
	return &ServerTunFactory{}
}

func (s ServerTunFactory) CreateTunDevice(connSettings settings.ConnectionSettings) (application.TunDevice, error) {
	tunFile, err := tools_linux.SetupServerTun(connSettings)
	if err != nil {
		log.Fatalf("failed to open TUN interface: %v", err)
	}

	configureErr := tools_linux.Configure(tunFile)
	if configureErr != nil {
		return nil, fmt.Errorf("failed to configure a server: %s\n", configureErr)
	}

	return tunFile, nil
}

func (s ServerTunFactory) DisposeTunDevices(connSettings settings.ConnectionSettings) error {
	tun, openErr := ip.OpenTunByName(connSettings.InterfaceName)
	if openErr != nil {
		log.Fatalf("failed to open TUN interface by name: %v", openErr)
	}
	tools_linux.Unconfigure(tun)

	closeErr := tun.Close()
	if closeErr != nil {
		log.Fatalf("failed to close TUN device: %v", closeErr)
	}

	_, delErr := ip.LinkDel(connSettings.InterfaceName)
	if delErr != nil {
		return fmt.Errorf("error deleting TUN device: %v", delErr)
	}

	return nil
}
