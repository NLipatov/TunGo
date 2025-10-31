package tun_client

import (
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
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
	netshFactory := netsh.NewFactory(conf, PAL.NewExecCommander())
	netshHandle, netshHandleErr := netshFactory.CreateNetsh()
	if netshHandleErr != nil {
		return nil, netshHandleErr
	}
	routeFactory := route.NewFactory(PAL.NewExecCommander(), connectionSettings)
	routeHandle, routeHandleErr := routeFactory.CreateRoute()
	if routeHandleErr != nil {
		return nil, routeHandleErr
	}
	return &PlatformTunManager{
		conf:               conf,
		connectionSettings: connectionSettings,
		netsh:              netshHandle,
		ipConfig:           ipconfig.NewWrapper(PAL.NewExecCommander()),
		route:              routeHandle,
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
	// Best-effort cleanup for BOTH families to avoid stale per-family state
	// when the user switches between IPv4-only and IPv6-only configs.
	cmd := PAL.NewExecCommander()
	v4 := netsh.NewV4Wrapper(cmd)
	v6 := netsh.NewV6Wrapper(cmd)
	r4 := route.NewV4Wrapper(cmd)
	r6 := route.NewV6Wrapper(cmd)
	cleanup := func(conf settings.Settings) {
		if conf.InterfaceName == "" {
			return
		}
		// 1) Drop default & split routes for both families.
		_ = v4.DeleteDefaultRoute(conf.InterfaceName)
		_ = v4.DeleteDefaultSplitRoutes(conf.InterfaceName)
		_ = v6.DeleteDefaultRoute(conf.InterfaceName)
		_ = v6.DeleteDefaultSplitRoutes(conf.InterfaceName)
		// 2) Remove address on the interface (family-aware for cleaner logs).
		if ip := net.ParseIP(conf.InterfaceAddress); ip != nil {
			if ip.To4() != nil {
				_ = v4.DeleteAddress(conf.InterfaceName, conf.InterfaceAddress)
			} else {
				_ = v6.DeleteAddress(conf.InterfaceName, conf.InterfaceAddress)
			}
		} else {
			// If the config carried a malformed or empty address, try both.
			_ = v4.DeleteAddress(conf.InterfaceName, conf.InterfaceAddress)
			_ = v6.DeleteAddress(conf.InterfaceName, conf.InterfaceAddress)
		}
		// 3) Remove host route to the server in both families (one will no-op).
		if conf.ConnectionIP != "" {
			_ = r4.Delete(conf.ConnectionIP)
			_ = r6.Delete(conf.ConnectionIP)
		}
		// 4) Reset DNS for both families (IPv4 → DHCP, IPv6 → clear list).
		_ = v4.SetDNS(conf.InterfaceName, nil)
		_ = v6.SetDNS(conf.InterfaceName, nil)
		// Note: MTU/metrics are not force-reset here intentionally to keep KISS.
	}
	cleanup(m.conf.TCPSettings)
	cleanup(m.conf.UDPSettings)
	cleanup(m.conf.WSSettings)
	return nil
}

func (m *PlatformTunManager) disposeDevice(conf settings.Settings) {
	_ = m.netsh.DeleteDefaultRoute(conf.InterfaceName)
	_ = m.netsh.DeleteAddress(conf.InterfaceName, conf.InterfaceAddress)
	_ = m.netsh.DeleteDefaultSplitRoutes(conf.InterfaceName)
	_ = m.route.Delete(conf.ConnectionIP)
	_ = m.netsh.SetDNS(conf.InterfaceName, nil)
}

func (m *PlatformTunManager) configureWindowsTunNetsh(ifName, ifAddr, ifCIDR string, mtu int) error {
	ip := net.ParseIP(ifAddr)
	_, nw, _ := net.ParseCIDR(ifCIDR)
	if ip == nil || nw == nil || !nw.Contains(ip) {
		return fmt.Errorf("address %s not in %s", ifAddr, ifCIDR)
	}
	prefix, _ := nw.Mask.Size()
	if ip.To4() != nil { // IPv4
		mask := net.CIDRMask(prefix, 32)
		maskStr := net.IP(mask).String()
		if err := m.netsh.SetAddressStatic(ifName, ifAddr, maskStr); err != nil {
			return err
		}
		_ = m.netsh.DeleteDefaultRoute(ifName)
		_ = m.netsh.DeleteDefaultSplitRoutes(ifName)
		if err := m.netsh.AddDefaultSplitRoutes(ifName, 1); err != nil {
			return err
		}
	} else { // IPv6
		pfxStr := strconv.Itoa(prefix)
		if err := m.netsh.SetAddressStatic(ifName, ifAddr, pfxStr); err != nil {
			return err
		}
		_ = m.netsh.DeleteDefaultRoute(ifName)
		_ = m.netsh.DeleteDefaultSplitRoutes(ifName)
		if err := m.netsh.AddDefaultSplitRoutes(ifName, 1); err != nil {
			return err
		}
	}
	if err := m.netsh.SetMTU(ifName, mtu); err != nil {
		return err
	}
	return nil
}
