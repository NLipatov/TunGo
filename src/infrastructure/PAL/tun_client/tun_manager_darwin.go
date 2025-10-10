package tun_client

import (
	"fmt"
	"log"
	"strings"
	"tungo/application/network/tun"
	"tungo/infrastructure/PAL"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/darwin/network_tools/ifconfig"
	"tungo/infrastructure/PAL/darwin/network_tools/route"
	"tungo/infrastructure/PAL/darwin/tun_adapters"
	"tungo/infrastructure/settings"
)

// PlatformTunManager is the macOS-specific implementation of ClientManager.
type PlatformTunManager struct {
	conf     client.Configuration
	dev      tun_adapters.Adapter
	route    route.Contract
	ifConfig ifconfig.Contract
}

// NewPlatformTunManager constructs a new PlatformTunManager.
func NewPlatformTunManager(conf client.Configuration) (tun.ClientManager, error) {
	return &PlatformTunManager{
		conf:     conf,
		route:    route.NewWrapper(PAL.NewExecCommander()),
		ifConfig: ifconfig.NewWrapper(PAL.NewExecCommander()),
	}, nil
}

// CreateDevice creates, configures and returns a TUN interface wrapped in wgTunAdapter.
func (t *PlatformTunManager) CreateTunDevice() (tun.Device, error) {
	var s settings.Settings
	switch t.conf.Protocol {
	case settings.TCP:
		s = t.conf.TCPSettings
	case settings.UDP:
		s = t.conf.UDPSettings
	case settings.WS, settings.WSS:
		s = t.conf.WSSettings
	default:
		return nil, fmt.Errorf("unsupported protocol")
	}

	tunFactory := tun_adapters.NewDefaultFactory(t.ifConfig)
	dev, err := tunFactory.CreateTUN(s.MTU)
	if err != nil {
		return nil, fmt.Errorf("failed to create TUN: %w", err)
	}

	name, nameErr := dev.Name()
	if nameErr != nil {
		return nil, fmt.Errorf("could not resolve created tun name: %w", nameErr)
	}

	t.dev = dev
	fmt.Printf("created TUN interface: %s\n", name)

	// Use host address (InterfaceAddress) + prefix from InterfaceIPCIDR
	cidrPrefix := strings.Split(s.InterfaceIPCIDR, "/")[1]
	addrCIDR := fmt.Sprintf("%s/%s", s.InterfaceAddress, cidrPrefix)

	if linkAddrAddErr := t.ifConfig.LinkAddrAdd(name, addrCIDR); linkAddrAddErr != nil {
		return nil, fmt.Errorf("failed to assign IP to %s: %w", name, linkAddrAddErr)
	}
	fmt.Printf("assigned IP %s to %s\n", addrCIDR, name)

	if getErr := t.route.Get(s.ConnectionIP); getErr != nil {
		return nil, fmt.Errorf("failed to route to server: %w", getErr)
	}

	if addSplitErr := t.route.AddSplit(name); addSplitErr != nil {
		return nil, fmt.Errorf("failed to add split default routes: %w", addSplitErr)
	}
	fmt.Printf("added split default routes via %s\n", name)

	return tun_adapters.NewWgTunAdapter(dev), nil
}

// DisposeDevices removes routes and destroys TUN interfaces.
func (t *PlatformTunManager) DisposeTunDevices() error {
	if t.dev != nil {
		dev := t.dev

		if t.dev != nil {
			devCloseErr := dev.Close()
			if devCloseErr != nil {
				log.Printf("tun dev close error: %v", devCloseErr)
			}
		}

		devName, devNameErr := dev.Name()
		if devNameErr != nil {
			delSplitErr := t.route.DelSplit(devName)
			if delSplitErr != nil {
				log.Printf("%v", delSplitErr.Error())
			}
		}
	}

	// Delete explicit routes to servers
	t.deleteRoute("UDP", t.conf.UDPSettings.ConnectionIP)
	t.deleteRoute("TCP", t.conf.TCPSettings.ConnectionIP)

	// Delete default gateway route if present
	if gw, err := t.route.DefaultGateway(); err == nil {
		t.deleteRoute("default gateway", gw)
	}

	return nil
}

func (t *PlatformTunManager) deleteRoute(label, dest string) {
	if dest == "" {
		return
	}
	if err := t.route.Del(dest); err != nil {
		log.Printf("tun_manager failed to delete route (%s - %s): %v", label, dest, err)
	}
}
