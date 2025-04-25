package pal_factory

import (
	"fmt"
	"log"
	"tungo/PAL/linux"
	ip2 "tungo/PAL/linux/ip"
	"tungo/application"
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
	tun, openErr := ip2.OpenTunByName(connSettings.InterfaceName)
	if openErr != nil {
		log.Fatalf("failed to open TUN interface by name: %v", openErr)
	}
	linux.Unconfigure(tun)

	closeErr := tun.Close()
	if closeErr != nil {
		log.Fatalf("failed to close TUN device: %v", closeErr)
	}

	_, delErr := ip2.LinkDel(connSettings.InterfaceName)
	if delErr != nil {
		return fmt.Errorf("error deleting TUN device: %v", delErr)
	}

	return nil
}
