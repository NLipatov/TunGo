package tun_client

import (
	"fmt"
	"log"
	"net"
	"strings"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/darwin/network_tools/ifconfig"
	"tungo/infrastructure/PAL/darwin/network_tools/route"
	"tungo/infrastructure/PAL/darwin/utun"
	"tungo/infrastructure/settings"
)

type PlatformTunManager struct {
	conf        client.Configuration
	dev         utun.UTUN
	route       route.Contract
	ifConfig    ifconfig.Contract
	devName     string
	pinnedGWv4  string
	pinnedGWv6  string
	installedV4 bool
	installedV6 bool
}

func NewPlatformTunManager(conf client.Configuration) (tun.ClientManager, error) {
	return &PlatformTunManager{
		conf:     conf,
		route:    route.NewWrapper(PAL.NewExecCommander()),
		ifConfig: ifconfig.NewWrapper(PAL.NewExecCommander()),
	}, nil
}

func isIPv6Addr(s string) bool {
	if s == "" {
		return false
	}
	if i := strings.IndexByte(s, '%'); i >= 0 {
		s = s[:i]
	}
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() == nil
}

func isIPv4Addr(s string) bool {
	if s == "" {
		return false
	}
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() != nil
}

func (t *PlatformTunManager) CreateDevice() (tun.Device, error) {
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

	tunFactory := utun.NewDefaultFactory(t.ifConfig)
	dev, err := tunFactory.CreateTUN(s.MTU)
	if err != nil {
		return nil, fmt.Errorf("failed to create TUN: %w", err)
	}

	name, nameErr := dev.Name()
	if nameErr != nil {
		return nil, fmt.Errorf("could not resolve created tun name: %w", nameErr)
	}
	if gw, _ := t.route.DefaultGateway(); gw != "" {
		t.pinnedGWv4 = gw
	}
	if gw6, _ := t.route.DefaultGatewayV6(); gw6 != "" {
		t.pinnedGWv6 = gw6
	}
	t.dev = dev
	fmt.Printf("created TUN interface: %s\n", name)

	// ---------- IPv4 address (if present in settings) ----------
	// Derive v4 address from InterfaceAddress + prefix from InterfaceIPCIDR when it looks IPv4.
	if strings.Contains(s.InterfaceIPCIDR, ".") || isIPv4Addr(s.InterfaceAddress) {
		pfx := "32"
		if parts := strings.Split(s.InterfaceIPCIDR, "/"); len(parts) == 2 && strings.Contains(parts[0], ".") {
			pfx = parts[1]
		}
		addrCIDRv4 := fmt.Sprintf("%s/%s", s.InterfaceAddress, pfx)
		if err := t.ifConfig.LinkAddrAdd(name, addrCIDRv4); err != nil {
			return nil, fmt.Errorf("failed to assign IPv4 to %s: %w", name, err)
		}
		fmt.Printf("assigned IPv4 %s to %s\n", addrCIDRv4, name)
	}

	// ---------- IPv6 address (stable p2p default /128) ----------
	needV6Addr := strings.Contains(s.InterfaceIPCIDR, ":") || isIPv6Addr(s.InterfaceAddress)
	if needV6Addr {
		// If CIDR is provided in settings, use it; otherwise default to /128.
		var addrCIDRv6 string
		if strings.Contains(s.InterfaceIPCIDR, ":") && strings.Contains(s.InterfaceIPCIDR, "/") {
			addrCIDRv6 = s.InterfaceIPCIDR
		} else if isIPv6Addr(s.InterfaceAddress) {
			addrCIDRv6 = s.InterfaceAddress + "/128"
		}
		if addrCIDRv6 != "" {
			if err := t.ifConfig.LinkAddrAddV6(name, addrCIDRv6); err != nil {
				return nil, fmt.Errorf("failed to assign IPv6 to %s: %w", name, err)
			}
			fmt.Printf("assigned IPv6 %s to %s\n", addrCIDRv6, name)
		}
	}

	// Pin a working path to the server (works for v4 or v6).
	if err := t.route.Get(s.ConnectionIP); err != nil {
		return nil, fmt.Errorf("failed to route to server: %w", err)
	}

	// Decide which split routes to install.
	needV6Split := isIPv6Addr(s.ConnectionIP) || strings.Contains(s.InterfaceIPCIDR, ":") || isIPv6Addr(s.InterfaceAddress)
	needV4Split := isIPv4Addr(s.ConnectionIP) || strings.Contains(s.InterfaceIPCIDR, ".") || isIPv4Addr(s.InterfaceAddress)
	if !needV4Split && !needV6Split {
		needV4Split = true // safe default
	}

	if needV4Split {
		if err := t.route.AddSplit(name); err != nil {
			return nil, fmt.Errorf("failed to add IPv4 split default routes: %w", err)
		}
		t.installedV4 = true
		fmt.Printf("added IPv4 split default routes via %s\n", name)
	}
	if needV6Split {
		if err := t.route.AddSplitV6(name); err != nil {
			return nil, fmt.Errorf("failed to add IPv6 split default routes: %w", err)
		}
		t.installedV6 = true
		fmt.Printf("added IPv6 split default routes via %s\n", name)
	}

	return utun.NewDarwinTunDevice(dev), nil
}

// DisposeDevices removes routes and destroys TUN interfaces.
func (t *PlatformTunManager) DisposeDevices() error {
	if t.dev == nil {
		return nil
	}

	// Remove split routes BEFORE closing the device.
	devName, devNameErr := t.dev.Name()
	if devNameErr == nil {
		if t.installedV4 && devName != "" {
			_ = t.route.DelSplit(devName)
		}
		if t.installedV6 && devName != "" {
			_ = t.route.DelSplitV6(devName)
		}
	} else {
		log.Printf("could not get tun name: %v", devNameErr)
	}

	// Delete explicit routes to servers (if set in config)
	t.deleteRoute("UDP", t.conf.UDPSettings.ConnectionIP)
	t.deleteRoute("TCP", t.conf.TCPSettings.ConnectionIP)
	t.deleteRoute("WS", t.conf.WSSettings.ConnectionIP)

	// Delete pinned route to default gateway if present
	if t.pinnedGWv4 != "" {
		_ = t.route.Del(t.pinnedGWv4)
	}
	if t.pinnedGWv6 != "" {
		_ = t.route.Del(t.pinnedGWv6)
	}
	_ = t.dev.Close()
	t.dev = nil
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
