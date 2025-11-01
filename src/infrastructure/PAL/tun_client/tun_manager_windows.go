package tun_client

import (
	"net"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/windows/manager"
	"tungo/infrastructure/PAL/windows/network_tools/netsh"
	"tungo/infrastructure/PAL/windows/network_tools/route"
	"tungo/infrastructure/settings"
)

type PlatformTunManager struct {
	configuration      client.Configuration
	connectionSettings settings.Settings
	// manager is a backing tun.ClientManager implementation, which handles v4/v6 specific
	manager tun.ClientManager
}

func NewPlatformTunManager(
	configuration client.Configuration,
) (tun.ClientManager, error) {
	connectionSettings, connectionSettingsErr := configuration.ActiveSettings()
	if connectionSettingsErr != nil {
		return nil, connectionSettingsErr
	}
	commander := PAL.NewExecCommander()
	factory := manager.NewFactory(
		connectionSettings,
		commander,
		netsh.NewFactory(connectionSettings, commander),
		route.NewFactory(commander, connectionSettings),
	)
	concreteManager, concreteManagerErr := factory.Create()
	if concreteManagerErr != nil {
		return nil, concreteManagerErr
	}
	return &PlatformTunManager{
		configuration:      configuration,
		connectionSettings: connectionSettings,
		manager:            concreteManager,
	}, nil
}

func (m *PlatformTunManager) CreateDevice() (tun.Device, error) {
	return m.manager.CreateDevice()
}

func (m *PlatformTunManager) DisposeDevices() error {
	commander := PAL.NewExecCommander()
	routeFactory := route.NewFactory(commander, m.connectionSettings)
	v4Route := routeFactory.CreateRouteV4()
	v6Route := routeFactory.CreateRouteV6()
	netshFactory := netsh.NewFactory(m.connectionSettings, commander)
	v4Netsh := netshFactory.CreateNetshV4()
	v6Netsh := netshFactory.CreateNetshV6()
	// Best-effort cleanup for BOTH families to avoid stale per-family state
	// when the user switches between IPv4-only and IPv6-only configs.
	cleanup := func(conf settings.Settings) {
		if conf.InterfaceName == "" {
			return
		}
		// 1) Drop default & split routes for both families.
		_ = v4Netsh.DeleteDefaultRoute(conf.InterfaceName)
		_ = v4Netsh.DeleteDefaultSplitRoutes(conf.InterfaceName)
		_ = v6Netsh.DeleteDefaultRoute(conf.InterfaceName)
		_ = v6Netsh.DeleteDefaultSplitRoutes(conf.InterfaceName)
		// 2) Remove address on the interface (family-aware for cleaner logs).
		if ip := net.ParseIP(conf.InterfaceAddress); ip != nil {
			if ip.To4() != nil {
				_ = v4Netsh.DeleteAddress(conf.InterfaceName, conf.InterfaceAddress)
			} else {
				_ = v6Netsh.DeleteAddress(conf.InterfaceName, conf.InterfaceAddress)
			}
		} else {
			// If the config carried a malformed or empty address, try both.
			_ = v4Netsh.DeleteAddress(conf.InterfaceName, conf.InterfaceAddress)
			_ = v6Netsh.DeleteAddress(conf.InterfaceName, conf.InterfaceAddress)
		}
		// 3) Remove host route to the server in both families (one will no-op).
		if conf.ConnectionIP != "" {
			_ = v4Route.Delete(conf.ConnectionIP)
			_ = v6Route.Delete(conf.ConnectionIP)
		}
		// 4) Reset DNS for both families (IPv4 → DHCP, IPv6 → clear list).
		_ = v4Netsh.SetDNS(conf.InterfaceName, nil)
		_ = v6Netsh.SetDNS(conf.InterfaceName, nil)
		// Note: MTU/metrics are not force-reset here intentionally to keep KISS.
	}
	cleanup(m.configuration.TCPSettings)
	cleanup(m.configuration.UDPSettings)
	cleanup(m.configuration.WSSettings)
	return nil
}
