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
	netshFactory := netsh.NewFactory(connectionSettings, PAL.NewExecCommander())
	netshHandle, netshHandleErr := netshFactory.CreateNetsh()
	if netshHandleErr != nil {
		return nil, netshHandleErr
	}
	routeFactory := route.NewFactory(PAL.NewExecCommander(), connectionSettings)
	routeHandle, routeHandleErr := routeFactory.CreateRoute()
	if routeHandleErr != nil {
		return nil, routeHandleErr
	}
	iFaceV4 := net.ParseIP(connectionSettings.InterfaceAddress).To4() != nil
	serverAddrV4 := net.ParseIP(connectionSettings.ConnectionIP).To4() != nil
	if iFaceV4 != serverAddrV4 {
		ifFam, serverFam := 4, 4
		if !iFaceV4 {
			ifFam = 6
		}
		if !serverAddrV4 {
			serverFam = 6
		}
		return nil, fmt.Errorf("IP version mismatch: interface %s(IPv%d) vs server %s(IPv%d)",
			connectionSettings.InterfaceAddress,
			ifFam,
			connectionSettings.ConnectionIP,
			serverFam,
		)
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
	adapter, err := wintun.OpenAdapter(m.connectionSettings.InterfaceName)
	if err != nil {
		adapter, err = wintun.CreateAdapter(m.connectionSettings.InterfaceName, "TunGo", nil)
		if err != nil {
			return nil, fmt.Errorf("create/open adapter: %w", err)
		}
	}
	mtu := m.connectionSettings.MTU
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
	_ = m.route.Delete(m.connectionSettings.ConnectionIP) // best-effort
	if addRouteErr := m.netsh.AddHostRouteViaGateway(
		m.connectionSettings.ConnectionIP,
		physIfName,
		origPhysGateway,
		1,
	); addRouteErr != nil {
		_ = device.Close()
		return nil, fmt.Errorf("could not add static route to server: %w", addRouteErr)
	}
	if err = m.configureWindowsTunNetsh(
		m.connectionSettings.InterfaceName,
		m.connectionSettings.InterfaceAddress,
		m.connectionSettings.InterfaceIPCIDR,
		mtu,
	); err != nil {
		_ = m.route.Delete(m.connectionSettings.ConnectionIP)
		_ = device.Close()
		return nil, err
	}
	// ToDo: use dns from configuration
	dnsV4 := []string{"1.1.1.1", "8.8.8.8"}
	dnsV6 := []string{"2606:4700:4700::1111", "2001:4860:4860::8888"}
	if ip := net.ParseIP(m.connectionSettings.InterfaceAddress); ip != nil && ip.To4() == nil {
		if len(dnsV6) > 0 {
			_ = m.netsh.SetDNS(m.connectionSettings.InterfaceName, dnsV6)
		} else {
			_ = m.netsh.SetDNS(m.connectionSettings.InterfaceName, nil) // DHCP
		}
	} else {
		if len(dnsV4) > 0 {
			_ = m.netsh.SetDNS(m.connectionSettings.InterfaceName, dnsV4)
		} else {
			_ = m.netsh.SetDNS(m.connectionSettings.InterfaceName, nil) // DHCP
		}
	}
	_ = m.ipConfig.FlushDNS()
	log.Printf("tun device created, interface %s, mtu %d", m.connectionSettings.InterfaceName, mtu)
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

func (m *PlatformTunManager) configureWindowsTunNetsh(
	ifName, ifAddr, ifCIDR string,
	mtu int,
) error {
	ip := net.ParseIP(ifAddr)
	_, nw, _ := net.ParseCIDR(ifCIDR)
	if ip == nil || nw == nil || !nw.Contains(ip) {
		return fmt.Errorf("address %s not in %s", ifAddr, ifCIDR)
	}
	parts := strings.Split(ifCIDR, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid CIDR: %s", ifCIDR)
	}
	prefStr := parts[1]
	isV4 := ip.To4() != nil
	if isV4 {
		pfx, _ := strconv.Atoi(prefStr)
		mask := net.IP(net.CIDRMask(pfx, 32)).String() // dotted mask for v4
		if err := m.netsh.SetAddressStatic(ifName, ifAddr, mask); err != nil {
			return err
		}
	} else {
		// For v6 pass prefix length string, e.g. "64"
		if err := m.netsh.SetAddressStatic(ifName, ifAddr, prefStr); err != nil {
			return err
		}
	}
	_ = m.netsh.DeleteDefaultRoute(ifName)
	_ = m.netsh.DeleteDefaultSplitRoutes(ifName)
	if err := m.netsh.AddDefaultSplitRoutes(ifName, 1); err != nil {
		return err
	}
	if err := m.netsh.SetMTU(ifName, mtu); err != nil {
		return err
	}
	return nil
}
