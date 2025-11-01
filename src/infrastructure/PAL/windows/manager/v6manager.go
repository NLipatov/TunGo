//go:build windows

package manager

import (
	"fmt"
	"net"
	"strings"

	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL/windows"
	"tungo/infrastructure/PAL/windows/network_tools/ipconfig"
	"tungo/infrastructure/PAL/windows/network_tools/netsh"
	"tungo/infrastructure/PAL/windows/network_tools/route"
	"tungo/infrastructure/PAL/windows/wtun"
	"tungo/infrastructure/settings"

	"golang.zx2c4.com/wintun"
)

// v6Manager configures a Wintun adapter and the host stack for IPv6.
type v6Manager struct {
	s        settings.Settings
	netsh    netsh.Contract
	route    route.Contract
	ipConfig ipconfig.Contract
	tun      tun.Device
}

func newV6Manager(
	s settings.Settings,
	netsh netsh.Contract,
	route route.Contract,
	ipConfig ipconfig.Contract,
) *v6Manager {
	return &v6Manager{
		s:        s,
		netsh:    netsh,
		route:    route,
		ipConfig: ipConfig,
	}
}

// CreateDevice creates/configures the TUN adapter and system routes/DNS for IPv6.
// Safe order mirrors v4 with IPv6-specific details.
func (m *v6Manager) CreateDevice() (tun.Device, error) {
	if net.ParseIP(m.s.InterfaceAddress).To4() != nil {
		return nil, fmt.Errorf("v6Manager requires IPv6 InterfaceAddress, got %q", m.s.InterfaceAddress)
	}
	if net.ParseIP(m.s.ConnectionIP).To4() != nil {
		return nil, fmt.Errorf("v6Manager requires IPv6 ConnectionIP, got %q", m.s.ConnectionIP)
	}

	tunDev, err := m.createTunDevice()
	if err != nil {
		return nil, err
	}
	m.tun = tunDev

	if err := m.addStaticRouteToServer(); err != nil {
		_ = m.DisposeDevices()
		return nil, err
	}
	if err := m.assignIPToTunDevice(); err != nil {
		_ = m.DisposeDevices()
		return nil, err
	}
	if err := m.setRouteToTunDevice(); err != nil {
		_ = m.DisposeDevices()
		return nil, err
	}
	if err := m.setMTUToTunDevice(); err != nil {
		_ = m.DisposeDevices()
		return nil, err
	}
	if err := m.setDNSToTunDevice(); err != nil {
		_ = m.DisposeDevices()
		return nil, err
	}
	return m.tun, nil
}

func (m *v6Manager) createTunDevice() (tun.Device, error) {
	adapter, err := wintun.CreateAdapter(m.s.InterfaceName, windows.TunGoTunnelType, nil)
	if err != nil {
		if existing, openErr := wintun.OpenAdapter(m.s.InterfaceName); openErr == nil {
			return wtun.NewTUN(existing)
		}
		return nil, fmt.Errorf("create/open adapter: %w", err)
	}
	dev, devErr := wtun.NewTUN(adapter)
	if devErr != nil {
		_ = adapter.Close()
		return nil, devErr
	}
	return dev, nil
}

// addStaticRouteToServer ensures a host route (/128) for the tunnel server exists,
// choosing on-link if the server is in any local IPv6 prefix, otherwise via the
// default IPv6 gateway/interface.
func (m *v6Manager) addStaticRouteToServer() error {
	gateway, ifName, _, err := m.route.DefaultRoute()
	if err != nil {
		return err
	}
	_ = m.route.Delete(m.s.ConnectionIP)

	srvIP := net.ParseIP(m.s.ConnectionIP)
	if alt, ok := m.onLinkInterfaceName(srvIP); ok {
		if err := m.netsh.AddHostRouteOnLink(m.s.ConnectionIP, alt, 1); err != nil {
			return fmt.Errorf("add on-link host route: %w", err)
		}
		return nil
	}
	if gateway != "" {
		if err := m.netsh.AddHostRouteViaGateway(m.s.ConnectionIP, ifName, gateway, 1); err != nil {
			return fmt.Errorf("add host route via gw: %w", err)
		}
		return nil
	}
	// No explicit gateway (ICS/bridges cases) — best-effort on-link via default IF.
	if err := m.netsh.AddHostRouteOnLink(m.s.ConnectionIP, ifName, 1); err != nil {
		return fmt.Errorf("add on-link host route (fallback): %w", err)
	}
	return nil
}

// onLinkInterfaceName returns the name of an interface whose IPv6 prefix contains 'server'.
func (m *v6Manager) onLinkInterfaceName(server net.IP) (string, bool) {
	if server == nil || server.To4() != nil {
		return "", false
	}
	ifaces, _ := net.Interfaces()
	for _, it := range ifaces {
		if (it.Flags & net.FlagUp) == 0 {
			continue
		}
		addrs, _ := it.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() == nil && ipn.Contains(server) {
				return it.Name, true
			}
		}
	}
	return "", false
}

// assignIPToTunDevice validates IPv6 address ∈ CIDR and applies it via prefix length.
func (m *v6Manager) assignIPToTunDevice() error {
	ip := net.ParseIP(m.s.InterfaceAddress)
	_, nw, _ := net.ParseCIDR(m.s.InterfaceIPCIDR)
	if ip == nil || ip.To4() != nil || nw == nil || !nw.Contains(ip) {
		_ = m.route.Delete(m.s.ConnectionIP)
		return fmt.Errorf("address %s not in %s", m.s.InterfaceAddress, m.s.InterfaceIPCIDR)
	}
	parts := strings.Split(m.s.InterfaceIPCIDR, "/")
	if len(parts) != 2 {
		_ = m.route.Delete(m.s.ConnectionIP)
		return fmt.Errorf("invalid IPv6 CIDR: %s", m.s.InterfaceIPCIDR)
	}
	prefix := parts[1]
	if err := m.netsh.SetAddressStatic(m.s.InterfaceName, m.s.InterfaceAddress, prefix); err != nil {
		_ = m.route.Delete(m.s.ConnectionIP)
		return err
	}
	return nil
}

// setRouteToTunDevice replaces any existing default with IPv6 split default (::/1, 8000::/1).
func (m *v6Manager) setRouteToTunDevice() error {
	_ = m.netsh.DeleteDefaultRoute(m.s.InterfaceName)
	_ = m.netsh.DeleteDefaultSplitRoutes(m.s.InterfaceName)
	if err := m.netsh.AddDefaultSplitRoutes(m.s.InterfaceName, 1); err != nil {
		_ = m.route.Delete(m.s.ConnectionIP)
		return err
	}
	return nil
}

func (m *v6Manager) setMTUToTunDevice() error {
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

func (m *v6Manager) setDNSToTunDevice() error {
	if err := m.netsh.SetDNS(m.s.InterfaceName,
		[]string{"2606:4700:4700::1111", "2001:4860:4860::8888"},
	); err != nil {
		_ = m.route.Delete(m.s.ConnectionIP)
		return err
	}
	_ = m.ipConfig.FlushDNS()
	return nil
}

// DisposeDevices reverses CreateDevice in safe order.
func (m *v6Manager) DisposeDevices() error {
	_ = m.route.Delete(m.s.ConnectionIP)
	if m.tun != nil {
		_ = m.tun.Close()
	}
	return nil
}
