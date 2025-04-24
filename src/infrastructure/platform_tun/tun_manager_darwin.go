package platform_tun

import (
	"fmt"
	"log"
	"os/exec"
	"strings"

	"golang.zx2c4.com/wireguard/tun"
	"tungo/application"
	"tungo/infrastructure/platform_tun/tools_darwin"
	"tungo/infrastructure/platform_tun/tools_darwin/ip"
	"tungo/settings"
	"tungo/settings/client_configuration"
)

// PlatformTunManager is the macOS-specific implementation of TunManager.
type PlatformTunManager struct {
	conf    client_configuration.Configuration
	devName string
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

	name, _ := dev.Name()
	t.devName = name
	fmt.Printf("created TUN interface: %s\n", name)

	// Use host address (InterfaceAddress) + prefix from InterfaceIPCIDR
	cidrPrefix := strings.Split(s.InterfaceIPCIDR, "/")[1]
	addrCIDR := fmt.Sprintf("%s/%s", s.InterfaceAddress, cidrPrefix)

	if err := ip.LinkAddrAdd(name, addrCIDR); err != nil {
		return nil, fmt.Errorf("failed to assign IP to %s: %w", name, err)
	}
	fmt.Printf("assigned IP %s to %s\n", addrCIDR, name)

	if err := ip.RouteAddToServer(s.ConnectionIP); err != nil {
		return nil, fmt.Errorf("failed to route to server: %w", err)
	}

	if err := ip.RouteAddSplit(name); err != nil {
		return nil, fmt.Errorf("failed to add split default routes: %w", err)
	}
	fmt.Printf("added split default routes via %s\n", name)

	return tools_darwin.NewWgTunAdapter(dev), nil
}

// DisposeTunDevices removes routes and destroys TUN interfaces.
func (t *PlatformTunManager) DisposeTunDevices() error {
	ip.RouteDelSplit(t.devName)
	_ = ip.RouteDel(t.conf.UDPSettings.ConnectionIP)
	_ = ip.RouteDel(t.conf.TCPSettings.ConnectionIP)
	_ = ip.LinkDel(t.conf.UDPSettings.InterfaceName)
	_ = ip.LinkDel(t.conf.TCPSettings.InterfaceName)
	if gw, err := defaultGateway(); err == nil {
		log.Printf("Default %s deleted", gw)
		_ = ip.RouteDel(gw)
	}
	return nil
}

// defaultGateway queries `route -n get default` to find the LAN gateway IP.
func defaultGateway() (string, error) {
	out, err := exec.Command("route", "-n", "get", "default").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("defaultGateway: %v (%s)", err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "gateway:" {
			return fields[1], nil
		}
	}
	return "", fmt.Errorf("defaultGateway: no gateway found")
}
