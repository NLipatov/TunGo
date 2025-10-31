package tun_client

import (
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/windows/network_tools/ipconfig"
	"tungo/infrastructure/PAL/windows/network_tools/netsh"
	"tungo/infrastructure/PAL/windows/network_tools/route"
	"tungo/infrastructure/PAL/windows/wtun"
	"tungo/infrastructure/settings"

	"golang.zx2c4.com/wintun"
)

type PlatformTunManager struct {
	conf               client.Configuration
	connectionSettings settings.Settings
	netsh              netsh.Contract
	ipConfig           ipconfig.Contract
	route              route.Contract
}

func NewPlatformTunManager(
	conf client.Configuration,
) (tun.ClientManager, error) {
	connectionSettings, connectionSettingsErr := settingsToUse(conf)
	if connectionSettingsErr != nil {
		return nil, connectionSettingsErr
	}
	nsFactory := netsh.NewFactory(conf, PAL.NewExecCommander())
	netshImpl, nsErr := nsFactory.CreateNetsh()
	if nsErr != nil {
		return nil, nsErr
	}
	rFactory := route.NewFactory(PAL.NewExecCommander(), connectionSettings)
	routeImpl, rErr := rFactory.CreateRoute()
	if rErr != nil {
		return nil, rErr
	}
	return &PlatformTunManager{
		conf:               conf,
		connectionSettings: connectionSettings,
		netsh:              netshImpl,
		ipConfig:           ipconfig.NewWrapper(PAL.NewExecCommander()),
		route:              routeImpl,
	}, nil
}

func settingsToUse(configuration client.Configuration) (settings.Settings, error) {
	var zero settings.Settings
	switch configuration.Protocol {
	case settings.UDP:
		return configuration.UDPSettings, nil
	case settings.TCP:
		return configuration.TCPSettings, nil
	case settings.WS, settings.WSS:
		return configuration.WSSettings, nil
	default:
		return zero, errors.New("unsupported protocol")
	}
}

func (m *PlatformTunManager) CreateDevice() (tun.Device, error) {
	connectionSettings, connectionSettingsErr := settingsToUse(m.conf)
	if connectionSettingsErr != nil {
		return nil, connectionSettingsErr
	}
	adapter, err := wintun.OpenAdapter(connectionSettings.InterfaceName)
	if err != nil {
		adapter, err = wintun.CreateAdapter(connectionSettings.InterfaceName, "TunGo", nil)
		if err != nil {
			return nil, fmt.Errorf("create/open adapter: %w", err)
		}
	}

	mtu := connectionSettings.MTU
	if mtu == 0 {
		mtu = settings.SafeMTU
	}

	device, err := wtun.NewTUN(adapter)
	if err != nil {
		_ = adapter.Close()
		return nil, err
	}
	origPhysGateway, physIfName, _, err := m.route.DefaultRoute()
	if err != nil {
		_ = adapter.Close()
		return nil, err
	}
	_ = m.route.Delete(connectionSettings.ConnectionIP) // best-effort
	if addRouteErr := m.netsh.AddHostRouteViaGateway(
		connectionSettings.ConnectionIP,
		physIfName,
		origPhysGateway,
		1,
	); addRouteErr != nil {
		_ = device.Close()
		return nil, fmt.Errorf("could not add static route to server: %w", addRouteErr)
	}
	if err = m.configureWindowsTunNetsh(
		connectionSettings.InterfaceName,
		connectionSettings.InterfaceAddress,
		connectionSettings.InterfaceIPCIDR,
		mtu,
	); err != nil {
		_ = m.route.Delete(connectionSettings.ConnectionIP)
		_ = device.Close()
		return nil, err
	}

	// ToDo: use dns from configuration
	dnsServers := []string{"1.1.1.1", "8.8.8.8"}
	if len(dnsServers) > 0 {
		if err = m.netsh.SetDNS(connectionSettings.InterfaceName, dnsServers); err != nil {
			_ = device.Close()
			return nil, err
		}
		_ = m.ipConfig.FlushDNS()
	} else {
		_ = m.netsh.SetDNS(connectionSettings.InterfaceName, nil) // DHCP
	}

	log.Printf("tun device created, interface %s, mtu %d", connectionSettings.InterfaceName, mtu)
	return device, nil
}

func (m *PlatformTunManager) DisposeDevices() error {
	m.disposeDevice(m.conf.TCPSettings)
	m.disposeDevice(m.conf.UDPSettings)
	m.disposeDevice(m.conf.WSSettings)
	return nil
}

func (m *PlatformTunManager) disposeDevice(conf settings.Settings) {
	_ = m.netsh.DeleteDefaultRoute(conf.InterfaceName)
	_ = m.netsh.DeleteAddress(conf.InterfaceName, conf.InterfaceAddress)
	_ = m.netsh.DeleteDefaultSplitRoutes(conf.InterfaceName)
	_ = m.route.Delete(conf.ConnectionIP)
	_ = m.netsh.SetDNS(conf.InterfaceName, nil)
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
	if err := m.netsh.SetAddressStatic(interfaceName, interfaceAddress, maskStr); err != nil {
		return err
	}
	_ = m.netsh.DeleteDefaultRoute(interfaceName)
	_ = m.netsh.DeleteDefaultSplitRoutes(interfaceName)
	if err := m.netsh.AddDefaultSplitRoutes(interfaceName, 1); err != nil {
		return err
	}
	if err := m.netsh.SetMTU(interfaceName, mtu); err != nil {
		return err
	}

	return nil
}
