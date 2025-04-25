package tun_manager

import (
	"fmt"
	"golang.zx2c4.com/wireguard/tun"
	"log"
	"strings"
	"tungo/PAL/darwin"
	"tungo/PAL/darwin/ip"
	"tungo/PAL/darwin/route"
	"tungo/application"
	"tungo/settings"
	"tungo/settings/client_configuration"
)

// PlatformTunManager is the macOS-specific implementation of TunManager.
type PlatformTunManager struct {
	conf client_configuration.Configuration
	dev  *tun.Device
}

// NewPlatformTunManager constructs a new PlatformTunManager.
func NewPlatformTunManager(conf client_configuration.Configuration) (application.TunManager, error) {
	return &PlatformTunManager{conf: conf}, nil
}

// CreateTunDevice creates, configures and returns a TUN interface wrapped in wgTunAdapter.
func (t *PlatformTunManager) CreateTunDevice() (application.TunDevice, error) {
	var s settings.ConnectionSettings
	switch t.conf.Protocol {
	case settings.TCP:
		s = t.conf.TCPSettings
	case settings.UDP:
		s = t.conf.UDPSettings
	default:
		return nil, fmt.Errorf("unsupported protocol")
	}

	dev, err := tun.CreateTUN("utun", s.MTU)
	if err != nil {
		return nil, fmt.Errorf("failed to create TUN: %w", err)
	}

	name, nameErr := dev.Name()
	if nameErr != nil {
		return nil, fmt.Errorf("could not resolve created tun name: %w", nameErr)
	}

	t.dev = &dev
	fmt.Printf("created TUN interface: %s\n", name)

	// Use host address (InterfaceAddress) + prefix from InterfaceIPCIDR
	cidrPrefix := strings.Split(s.InterfaceIPCIDR, "/")[1]
	addrCIDR := fmt.Sprintf("%s/%s", s.InterfaceAddress, cidrPrefix)

	if linkAddrAddErr := ip.LinkAddrAdd(name, addrCIDR); linkAddrAddErr != nil {
		return nil, fmt.Errorf("failed to assign IP to %s: %w", name, linkAddrAddErr)
	}
	fmt.Printf("assigned IP %s to %s\n", addrCIDR, name)

	if getErr := route.Get(s.ConnectionIP); getErr != nil {
		return nil, fmt.Errorf("failed to route to server: %w", getErr)
	}

	if addSplitErr := route.AddSplit(name); addSplitErr != nil {
		return nil, fmt.Errorf("failed to add split default routes: %w", addSplitErr)
	}
	fmt.Printf("added split default routes via %s\n", name)

	return darwin.NewWgTunAdapter(dev), nil
}

// DisposeTunDevices removes routes and destroys TUN interfaces.
func (t *PlatformTunManager) DisposeTunDevices() error {
	if t.dev != nil {
		dev := *t.dev

		if t.dev != nil {
			devCloseErr := dev.Close()
			if devCloseErr != nil {
				log.Printf("tun dev close error: %v", devCloseErr)
			}
		}

		devName, devNameErr := dev.Name()
		if devNameErr != nil {
			delSplitErr := route.DelSplit(devName)
			if delSplitErr != nil {
				log.Printf(delSplitErr.Error())
			}
		}
	}

	// Delete explicit routes to servers
	deleteRoute("UDP", t.conf.UDPSettings.ConnectionIP)
	deleteRoute("TCP", t.conf.TCPSettings.ConnectionIP)

	// Delete default gateway route if present
	if gw, err := route.DefaultGateway(); err == nil {
		deleteRoute("default gateway", gw)
	}

	return nil
}

func deleteRoute(label, dest string) {
	if dest == "" {
		return
	}
	if err := route.Del(dest); err != nil {
		log.Printf("tun_manager failed to delete route (%s - %s): %v", label, dest, err)
	}
}
