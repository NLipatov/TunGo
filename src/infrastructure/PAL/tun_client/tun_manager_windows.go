package tun_client

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/windows/network_tools/ipconfig"
	"tungo/infrastructure/PAL/windows/network_tools/netsh"
	"tungo/infrastructure/PAL/windows/wtun"
	"tungo/infrastructure/settings"

	"golang.zx2c4.com/wintun"
)

type PlatformTunManager struct {
	conf     client.Configuration
	netsh    netsh.Contract
	ipconfig ipconfig.Contract
}

func NewPlatformTunManager(
	conf client.Configuration,
) (tun.ClientManager, error) {
	return &PlatformTunManager{
		conf:     conf,
		netsh:    netsh.NewWrapper(PAL.NewExecCommander()),
		ipconfig: ipconfig.NewWrapper(PAL.NewExecCommander()),
	}, nil
}

func (m *PlatformTunManager) CreateDevice() (tun.Device, error) {
	var s settings.Settings
	switch m.conf.Protocol {
	case settings.UDP:
		s = m.conf.UDPSettings
	case settings.TCP:
		s = m.conf.TCPSettings
	case settings.WS, settings.WSS:
		s = m.conf.WSSettings
	default:
		return nil, errors.New("unsupported protocol")
	}
	origPhysGateway, origPhysIP, err := m.getOriginalPhysicalGatewayAndInterface()
	if err != nil {
		return nil, fmt.Errorf("original route error: %w", err)
	}
	adapter, err := wintun.OpenAdapter(s.InterfaceName)
	if err != nil {
		adapter, err = wintun.CreateAdapter(s.InterfaceName, "TunGo", nil)
		if err != nil {
			return nil, fmt.Errorf("create/open adapter: %w", err)
		}
	}

	mtu := s.MTU
	if mtu == 0 {
		mtu = settings.SafeMTU
	}

	device, err := wtun.NewTUN(adapter)
	if err != nil {
		_ = adapter.Close()
		return nil, err
	}

	_ = m.netsh.RouteDelete(s.ConnectionIP) // best-effort
	if err = addStaticRouteToServer(s.ConnectionIP, origPhysIP, origPhysGateway); err != nil {
		_ = device.Close()
		return nil, fmt.Errorf("could not add static route to server: %w", err)
	}
	if err = m.configureWindowsTunNetsh(
		s.InterfaceName,
		s.InterfaceAddress,
		s.InterfaceIPCIDR,
		mtu,
	); err != nil {
		_ = m.netsh.RouteDelete(s.ConnectionIP)
		_ = device.Close()
		return nil, err
	}

	// ToDo: use dns from configuration
	dnsServers := []string{"1.1.1.1", "8.8.8.8"}
	if len(dnsServers) > 0 {
		if err = m.netsh.InterfaceSetDNSServers(s.InterfaceName, dnsServers); err != nil {
			_ = device.Close()
			return nil, err
		}
		_ = m.ipconfig.FlushDNS()
	} else {
		_ = m.netsh.InterfaceSetDNSServers(s.InterfaceName, nil) // DHCP
	}

	log.Printf("tun device created, interface %s, mtu %d", s.InterfaceName, mtu)
	return device, nil
}

func (m *PlatformTunManager) DisposeDevices() error {
	m.disposeDevice(m.conf.TCPSettings)
	m.disposeDevice(m.conf.UDPSettings)
	m.disposeDevice(m.conf.WSSettings)
	return nil
}

func (m *PlatformTunManager) disposeDevice(conf settings.Settings) {
	_ = m.netsh.InterfaceDeleteDefaultRoute(conf.InterfaceName)
	_ = m.netsh.InterfaceIPDeleteAddress(conf.InterfaceName, conf.InterfaceAddress)
	_ = m.netsh.InterfaceDeleteRoute("0.0.0.0/1", conf.InterfaceName)
	_ = m.netsh.InterfaceDeleteRoute("128.0.0.0/1", conf.InterfaceName)
	_ = m.netsh.RouteDelete(conf.ConnectionIP)
	_ = m.netsh.InterfaceSetDNSServers(conf.InterfaceName, nil)
}

func (m *PlatformTunManager) configureWindowsTunNetsh(
	interfaceName, interfaceAddress, InterfaceIPCIDR string,
	mtu int,
) error {
	ip := net.ParseIP(interfaceAddress)
	_, nw, _ := net.ParseCIDR(InterfaceIPCIDR)
	if ip == nil || nw == nil || !nw.Contains(ip) {
		return fmt.Errorf("address %s not in %s", interfaceAddress, InterfaceIPCIDR)
	}
	parts := strings.Split(InterfaceIPCIDR, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid CIDR: %s", InterfaceIPCIDR)
	}
	prefix, _ := strconv.Atoi(parts[1])
	mask := net.CIDRMask(prefix, 32)
	maskStr := net.IP(mask).String()

	// Wintun: address on-link (no gateway)
	if err := m.netsh.InterfaceSetAddressNoGateway(interfaceName, interfaceAddress, maskStr); err != nil {
		return err
	}
	_ = m.netsh.InterfaceDeleteDefaultRoute(interfaceName)
	_ = m.netsh.InterfaceDeleteRoute("0.0.0.0/1", interfaceName)
	_ = m.netsh.InterfaceDeleteRoute("128.0.0.0/1", interfaceName)
	if err := m.netsh.InterfaceAddRouteOnLink("0.0.0.0/1", interfaceName, 1); err != nil {
		return err
	}
	if err := m.netsh.InterfaceAddRouteOnLink("128.0.0.0/1", interfaceName, 1); err != nil {
		return err
	}
	if err := m.netsh.LinkSetDevMTU(interfaceName, mtu); err != nil {
		return err
	}

	return nil
}

func (m *PlatformTunManager) getOriginalPhysicalGatewayAndInterface() (gateway, ifaceIP string, err error) {
	out, err := exec.Command("route", "print", "0.0.0.0").CombinedOutput()
	if err != nil {
		return
	}
	lines := strings.Split(string(out), "\n")
	bestMetric := int(^uint(0) >> 1)
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 5 && fields[0] == "0.0.0.0" {
			metric, _ := strconv.Atoi(fields[4])
			if metric < bestMetric {
				bestMetric = metric
				gateway, ifaceIP = fields[2], fields[3]
			}
		}
	}
	if gateway == "" || ifaceIP == "" {
		err = errors.New("default route not found")
	}
	return
}

func addStaticRouteToServer(serverIP, physIP, physGateway string) error {
	idx := getInterfaceIndexByIP(physIP)
	if idx == 0 {
		return fmt.Errorf("could not find interface index for %s", physIP)
	}
	interfaceByIndex, err := net.InterfaceByIndex(idx)
	if err != nil {
		return err
	}
	cmd := exec.Command("route", "add", serverIP, "mask", "255.255.255.255",
		physGateway, "metric", "1", "if", strconv.Itoa(interfaceByIndex.Index))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("route add host %s via %s if %d: %v, output: %s",
			serverIP, physGateway, interfaceByIndex.Index, err, string(out))
	}
	return nil
}

func getInterfaceIndexByIP(ip string) int {
	want := net.ParseIP(ip)
	if want == nil {
		return 0
	}
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			if ipn, ok := addr.(*net.IPNet); ok && ipn.IP.Equal(want) {
				return iface.Index
			}
		}
	}
	return 0
}
