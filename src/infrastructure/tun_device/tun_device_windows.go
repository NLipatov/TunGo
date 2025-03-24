//go:build windows
// +build windows

package tun_device

import (
	"fmt"
	"os/exec"
	"strings"
	"tungo/application"
	"tungo/settings"
	"tungo/settings/client_configuration"

	"github.com/songgao/water"
)

type originalRoute struct {
	Gateway string
	IfIndex string
}

type windowsTunDeviceManager struct {
	conf          client_configuration.Configuration
	iface         *water.Interface
	originalRoute *originalRoute
}

func newPlatformTunConfigurator(conf client_configuration.Configuration) (application.PlatformTunConfigurator, error) {
	origRoute, err := getDefaultRoute()
	if err != nil {
		return nil, err
	}

	return &windowsTunDeviceManager{
		conf:          conf,
		originalRoute: origRoute,
	}, nil
}

func (t *windowsTunDeviceManager) CreateTunDevice() (application.TunDevice, error) {
	var s settings.ConnectionSettings
	switch t.conf.Protocol {
	case settings.UDP:
		s = t.conf.UDPSettings
	case settings.TCP:
		s = t.conf.TCPSettings
	default:
		return nil, fmt.Errorf("unsupported protocol")
	}

	config := water.Config{DeviceType: water.TUN}
	config.ComponentID = "tap0901" // Используем стандартный TAP-драйвер
	config.Network = s.InterfaceIPCIDR

	ifce, err := water.New(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create TUN device: %v", err)
	}

	t.iface = ifce

	// Configure interface
	if err := configureTUNWindows(s, ifce.Name()); err != nil {
		return nil, err
	}

	return ifce, nil
}

func (t *windowsTunDeviceManager) DisposeTunDevices() error {
	// Remove VPN-added routes
	routeDelErr := exec.Command("route", "delete", "0.0.0.0").Run()
	routeDelErr = exec.Command("route", "delete", t.conf.UDPSettings.ConnectionIP).Run()
	routeDelErr = exec.Command("route", "delete", t.conf.TCPSettings.ConnectionIP).Run()
	if routeDelErr != nil {
		return routeDelErr
	}

	// Restore original default route
	if t.originalRoute != nil {
		cmd := exec.Command("route", "add", "0.0.0.0", "mask", "0.0.0.0",
			t.originalRoute.Gateway, "metric", "1", "if", t.originalRoute.IfIndex)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to restore default route: %v\n%s", err, out)
		}
	}

	// Flush DNS
	flushDnsErr := exec.Command("ipconfig", "/flushdns").Run()
	if flushDnsErr != nil {
		return flushDnsErr
	}

	return nil
}

func getDefaultRoute() (*originalRoute, error) {
	cmd := exec.Command("powershell", "-Command", `(Get-NetRoute -DestinationPrefix "0.0.0.0/0" | Sort-Object RouteMetric | Select-Object -First 1) | Format-List`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get default route: %v\n%s", err, out)
	}

	lines := strings.Split(string(out), "\n")
	route := &originalRoute{}
	for _, line := range lines {
		if strings.Contains(line, "NextHop") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				route.Gateway = strings.TrimSpace(parts[1])
			}
		}
		if strings.Contains(line, "InterfaceIndex") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				route.IfIndex = strings.TrimSpace(parts[1])
			}
		}
	}
	if route.Gateway == "" || route.IfIndex == "" {
		return nil, fmt.Errorf("failed to parse default route")
	}
	return route, nil
}

func configureTUNWindows(s settings.ConnectionSettings, ifName string) error {
	ip := strings.Split(s.InterfaceIPCIDR, "/")[0]

	cmd := exec.Command("netsh", "interface", "ip", "set", "address", fmt.Sprintf("name=%s", ifName), "static", ip, "255.255.255.0", "0.0.0.0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("set IP error: %v\n%s", err, out)
	}

	cmd = exec.Command("netsh", "interface", "ipv4", "set", "subinterface", ifName, fmt.Sprintf("mtu=%d", s.MTU), "store=persistent")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("set MTU error: %v\n%s", err, out)
	}

	cmd = exec.Command("route", "add", s.ConnectionIP, "mask", "255.255.255.255", ip)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("add route error: %v\n%s", err, out)
	}

	cmd = exec.Command("route", "add", "0.0.0.0", "mask", "0.0.0.0", ip)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("add default route error: %v\n%s", err, out)
	}

	return nil
}
