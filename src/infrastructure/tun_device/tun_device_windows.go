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

type originalDNS struct {
	Interface string
	DNS       string
}

type windowsTunDeviceManager struct {
	conf          client_configuration.Configuration
	iface         *water.Interface
	originalRoute *originalRoute
	originalDNS   *originalDNS
}

func newPlatformTunConfigurator(conf client_configuration.Configuration) (application.PlatformTunConfigurator, error) {
	origRoute, err := getDefaultRoute()
	if err != nil {
		return nil, err
	}

	origDNS, err := getOriginalDNS()
	if err != nil {
		return nil, err
	}

	return &windowsTunDeviceManager{
		conf:          conf,
		originalRoute: origRoute,
		originalDNS:   origDNS,
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
	config.ComponentID = "tap0901"
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
	_ = exec.Command("route", "delete", "0.0.0.0").Run()
	_ = exec.Command("route", "delete", t.conf.UDPSettings.ConnectionIP).Run()
	_ = exec.Command("route", "delete", "1.1.1.1").Run()

	// Restore original default route
	if t.originalRoute != nil {
		_ = exec.Command("route", "add", "0.0.0.0", "mask", "0.0.0.0",
			t.originalRoute.Gateway, "metric", "1", "if", t.originalRoute.IfIndex).Run()
	}

	// Restore original DNS
	if t.originalDNS != nil {
		_ = exec.Command("netsh", "interface", "ipv4", "set", "dnsservers",
			fmt.Sprintf(`name="%s"`, t.originalDNS.Interface),
			"static", t.originalDNS.DNS, "primary").Run()
	}

	_ = exec.Command("ipconfig", "/flushdns").Run()

	return nil
}

func getDefaultRoute() (*originalRoute, error) {
	cmd := exec.Command("powershell", "-Command",
		`Get-NetRoute -DestinationPrefix "0.0.0.0/0" | Sort-Object RouteMetric | Select-Object -First 1 | Format-List`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get default route: %v\n%s", err, out)
	}

	lines := strings.Split(string(out), "\n")
	route := &originalRoute{}
	for _, line := range lines {
		if strings.Contains(line, "NextHop") {
			route.Gateway = strings.TrimSpace(strings.Split(line, ":")[1])
		}
		if strings.Contains(line, "InterfaceIndex") {
			route.IfIndex = strings.TrimSpace(strings.Split(line, ":")[1])
		}
	}
	if route.Gateway == "" || route.IfIndex == "" {
		return nil, fmt.Errorf("failed to parse default route")
	}
	return route, nil
}

func getOriginalDNS() (*originalDNS, error) {
	cmd := exec.Command("powershell", "-Command",
		`Get-DnsClientServerAddress -AddressFamily IPv4 | Where-Object {$_.ServerAddresses} | Select-Object -First 1 | Format-List`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get original DNS: %v\n%s", err, out)
	}

	lines := strings.Split(string(out), "\n")
	dns := &originalDNS{}
	for _, line := range lines {
		if strings.Contains(line, "InterfaceAlias") {
			dns.Interface = strings.TrimSpace(strings.Split(line, ":")[1])
		}
		if strings.Contains(line, "ServerAddresses") {
			addresses := strings.TrimSpace(strings.Split(line, ":")[1])
			dns.DNS = strings.Fields(addresses)[0]
		}
	}
	if dns.Interface == "" || dns.DNS == "" {
		return nil, fmt.Errorf("failed to parse original DNS")
	}
	return dns, nil
}

func configureTUNWindows(s settings.ConnectionSettings, ifName string) error {
	ip := strings.Split(s.InterfaceIPCIDR, "/")[0]

	if _, err := exec.Command("netsh", "interface", "ip", "set", "address",
		fmt.Sprintf("name=%s", ifName), "static", ip, "255.255.255.0", "0.0.0.0").CombinedOutput(); err != nil {
		return err
	}

	if _, err := exec.Command("netsh", "interface", "ipv4", "set", "subinterface",
		fmt.Sprintf(`"%s"`, ifName), fmt.Sprintf("mtu=%d", s.MTU), "store=persistent").CombinedOutput(); err != nil {
		return err
	}

	_ = exec.Command("route", "add", s.ConnectionIP, "mask", "255.255.255.255", ip).Run()
	_ = exec.Command("route", "add", "0.0.0.0", "mask", "0.0.0.0", ip).Run()

	// Set DNS explicitly to 1.1.1.1 for speed
	if _, err := exec.Command("netsh", "interface", "ipv4", "set", "dnsservers",
		fmt.Sprintf(`name="%s"`, ifName), "static", "1.1.1.1", "primary").CombinedOutput(); err != nil {
		return err
	}

	// Route DNS explicitly
	_ = exec.Command("route", "add", "1.1.1.1", "mask", "255.255.255.255", ip).Run()

	return nil
}
