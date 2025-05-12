package tun_server

import (
	"fmt"
	"log"
	"tungo/application"
	"tungo/infrastructure/PAL/linux"
	"tungo/infrastructure/PAL/linux/ip"
	"tungo/infrastructure/PAL/linux/syscall"
	"tungo/settings"
)

type ServerTunFactory struct {
}

func NewServerTunFactory() application.ServerTunManager {
	return &ServerTunFactory{}
}

func (s ServerTunFactory) CreateTunDevice(connSettings settings.ConnectionSettings) (application.TunDevice, error) {
	tunFile, err := linux.SetupServerTun(connSettings)
	if err != nil {
		log.Fatalf("failed to open TUN interface: %v", err)
	}

	configureErr := linux.Configure(tunFile)
	if configureErr != nil {
		return nil, fmt.Errorf("failed to configure a server: %s\n", configureErr)
	}

	return tunFile, nil
}

func (s ServerTunFactory) DisposeTunDevices(connSettings settings.ConnectionSettings) error {
	tun, openErr := syscall.CreateTunInterface(connSettings.InterfaceName)
	if openErr != nil {
		log.Fatalf("failed to open TUN interface by name: %v", openErr)
	}
	linux.Unconfigure(tun)

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
