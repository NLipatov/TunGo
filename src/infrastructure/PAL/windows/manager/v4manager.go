//go:build windows

package manager

import (
	"fmt"
	"net"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL/windows/network_tools/ipconfig"
	"tungo/infrastructure/PAL/windows/network_tools/netsh"
	"tungo/infrastructure/PAL/windows/network_tools/route"
	"tungo/infrastructure/PAL/windows/wtun"
	"tungo/infrastructure/settings"

	"golang.zx2c4.com/wintun"
)

type v4Manager struct {
	s        settings.Settings
	netsh    netsh.Contract
	route    route.Contract
	ipConfig ipconfig.Contract
	tun      tun.Device
}

func newV4Manager(
	s settings.Settings,
	netsh netsh.Contract,
	route route.Contract,
	ipConfig ipconfig.Contract,
) *v4Manager {
	return &v4Manager{
		s:        s,
		netsh:    netsh,
		route:    route,
		ipConfig: ipConfig,
	}
}

func (m *v4Manager) CreateDevice() (tun.Device, error) {
	if net.ParseIP(m.s.InterfaceAddress).To4() == nil {
		return nil, fmt.Errorf("v4Manager requires IPv4 InterfaceAddress, got %q", m.s.InterfaceAddress)
	}
	if net.ParseIP(m.s.ConnectionIP).To4() == nil {
		return nil, fmt.Errorf("v4Manager requires IPv4 ConnectionIP, got %q", m.s.ConnectionIP)
	}
	tunDev, err := m.createTunDevice()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = m.DisposeDevices()
		}
	}()
	if err := m.addStaticRouteToServer(); err != nil {
		_ = tunDev.Close()
		return nil, err
	}
	if err := m.assignIPToTunDevice(); err != nil {
		_ = tunDev.Close()
		return nil, err
	}
	if err := m.setRouteToTunDevice(); err != nil {
		_ = tunDev.Close()
		return nil, err
	}
	if err := m.setMTUToTunDevice(); err != nil {
		_ = tunDev.Close()
		return nil, err
	}
	if err := m.setDNSToTunDevice(); err != nil {
		_ = tunDev.Close()
		return nil, err
	}
	m.tun = tunDev
	return m.tun, nil
}

func (m *v4Manager) createTunDevice() (tun.Device, error) {
	wintunAdapter, wintunAdapterErr := wintun.CreateAdapter(m.s.InterfaceName, "TunGo", nil)
	if wintunAdapterErr != nil {
		// if it already exists, fall back:
		if existingWintunAdapter, existingWintunAdapterErr := wintun.OpenAdapter(
			m.s.InterfaceName,
		); existingWintunAdapterErr == nil {
			return wtun.NewTUN(existingWintunAdapter)
		}
		return nil, fmt.Errorf("create/open adapter: %w", wintunAdapterErr)
	}
	tunDevice, tunDeviceErr := wtun.NewTUN(wintunAdapter)
	if tunDeviceErr != nil {
		_ = wintunAdapter.Close()
		return nil, tunDeviceErr
	}
	return tunDevice, nil
}

func (m *v4Manager) addStaticRouteToServer() error {
	// check what is default route and default interface
	gateway, routeInterface, _, defaultRouteErr := m.route.DefaultRoute()
	if defaultRouteErr != nil {
		return defaultRouteErr
	}
	// remove old route to server and assign new and actual one
	_ = m.route.Delete(m.s.ConnectionIP)
	serverIP := net.ParseIP(m.s.ConnectionIP)
	// is server on the same network (on-link route)?
	if altIFace, ok := m.onLinkInterfaceName(serverIP); ok {
		routeInterface = altIFace
		// use on-link route
		if err := m.netsh.AddHostRouteOnLink(m.s.ConnectionIP, routeInterface, 1); err != nil {
			return fmt.Errorf("add on-link host route: %w", err)
		}
	} else {
		// use off-link route(via gateway)
		if err := m.netsh.AddHostRouteViaGateway(m.s.ConnectionIP, routeInterface, gateway, 1); err != nil {
			return fmt.Errorf("add host route via gw: %w", err)
		}
	}
	return nil
}

func (m *v4Manager) onLinkInterfaceName(server net.IP) (string, bool) {
	srv4 := server.To4()
	if srv4 == nil {
		return "", false
	}
	iFaces, _ := net.Interfaces()
	for _, iFace := range iFaces {
		addresses, _ := iFace.Addrs()
		for _, a := range addresses {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil && ipn.Contains(srv4) {
				return iFace.Name, true
			}
		}
	}
	return "", false
}

func (m *v4Manager) assignIPToTunDevice() error {
	// check that m.s.InterfaceAddress is in m.s.InterfaceIPCIDR subnet
	ip := net.ParseIP(m.s.InterfaceAddress)
	_, nw, _ := net.ParseCIDR(m.s.InterfaceIPCIDR)
	if ip == nil || nw == nil || !nw.Contains(ip) {
		_ = m.route.Delete(m.s.ConnectionIP)
		return fmt.Errorf("address %s not in %s", m.s.InterfaceAddress, m.s.InterfaceIPCIDR)
	}
	mask := net.IP(nw.Mask).String() // dotted decimal mask
	if err := m.netsh.SetAddressStatic(m.s.InterfaceName, m.s.InterfaceAddress, mask); err != nil {
		_ = m.route.Delete(m.s.ConnectionIP)
		return err
	}
	return nil
}

func (m *v4Manager) setRouteToTunDevice() error {
	// (re-)set route to TUN device
	_ = m.netsh.DeleteDefaultRoute(m.s.InterfaceName)
	_ = m.netsh.DeleteDefaultSplitRoutes(m.s.InterfaceName)
	if err := m.netsh.AddDefaultSplitRoutes(m.s.InterfaceName, 1); err != nil {
		_ = m.route.Delete(m.s.ConnectionIP)
		return err
	}
	return nil
}

func (m *v4Manager) setMTUToTunDevice() error {
	mtu := m.s.MTU
	if mtu == 0 {
		mtu = settings.SafeMTU
	}
	if err := m.netsh.SetMTU(m.s.InterfaceName, mtu); err != nil {
		_ = m.route.Delete(m.s.ConnectionIP)
		return err
	}
	return nil
}

func (m *v4Manager) setDNSToTunDevice() error {
	if err := m.netsh.SetDNS(m.s.InterfaceName, []string{"1.1.1.1", "8.8.8.8"}); err != nil {
		return err
	}
	_ = m.ipConfig.FlushDNS()
	return nil
}

func (m *v4Manager) DisposeDevices() error {
	_ = m.netsh.DeleteDefaultRoute(m.s.InterfaceName)
	_ = m.netsh.DeleteDefaultSplitRoutes(m.s.InterfaceName)
	_ = m.netsh.DeleteAddress(m.s.InterfaceName, m.s.InterfaceAddress)
	_ = m.route.Delete(m.s.ConnectionIP)
	_ = m.netsh.SetDNS(m.s.InterfaceName, nil)
	if m.tun != nil {
		_ = m.tun.Close()
	}
	return nil
}
