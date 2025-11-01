//go:build windows

package manager

import (
	"fmt"
	"net"
	"strings"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL/windows/network_tools/ipconfig"
	"tungo/infrastructure/PAL/windows/network_tools/netsh"
	"tungo/infrastructure/PAL/windows/network_tools/route"
	"tungo/infrastructure/PAL/windows/wtun"
	"tungo/infrastructure/settings"

	"golang.zx2c4.com/wintun"
)

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

func (m *v6Manager) CreateDevice() (tun.Device, error) {
	// Guard: IPv6 only
	if net.ParseIP(m.s.InterfaceAddress).To4() != nil {
		return nil, fmt.Errorf("v6Manager requires IPv6 InterfaceAddress, got %q", m.s.InterfaceAddress)
	}
	if net.ParseIP(m.s.ConnectionIP).To4() != nil {
		return nil, fmt.Errorf("v6Manager requires IPv6 ConnectionIP, got %q", m.s.ConnectionIP)
	}
	// Create adapter first; if it exists already — fallback to open.
	tunDev, err := m.createTunDevice()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = m.DisposeDevices()
		}
	}()
	// Ensure static host route to server (on-link vs via gateway).
	if err := m.addStaticRouteToServer(); err != nil {
		_ = tunDev.Close()
		return nil, err
	}
	// Assign IPv6 address to TUN.
	if err := m.assignIPToTunDevice(); err != nil {
		_ = tunDev.Close()
		return nil, err
	}
	// Install split default (::/1, 8000::/1).
	if err := m.setRouteToTunDevice(); err != nil {
		_ = tunDev.Close()
		return nil, err
	}
	// MTU.
	if err := m.setMTUToTunDevice(); err != nil {
		_ = tunDev.Close()
		return nil, err
	}
	// DNS (IPv6 resolvers), with rollback on failure.
	if err := m.setDNSToTunDevice(); err != nil {
		_ = tunDev.Close()
		return nil, err
	}
	m.tun = tunDev
	return m.tun, nil
}

func (m *v6Manager) createTunDevice() (tun.Device, error) {
	adapter, err := wintun.CreateAdapter(m.s.InterfaceName, "TunGo", nil)
	if err != nil {
		// If adapter already exists, fall back to open.
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

func (m *v6Manager) addStaticRouteToServer() error {
	gateway, defIF, _, err := m.route.DefaultRoute()
	if err != nil {
		return err
	}
	// Refresh host route to server.
	_ = m.route.Delete(m.s.ConnectionIP)
	srvIP := net.ParseIP(m.s.ConnectionIP)
	if altIF, ok := m.onLinkInterfaceName(srvIP); ok {
		// On-link host route (no nexthop)
		if err := m.netsh.AddHostRouteOnLink(m.s.ConnectionIP, altIF, 1); err != nil {
			return fmt.Errorf("add on-link host route: %w", err)
		}
	} else if gateway != "" {
		// Off-link via gateway
		if err := m.netsh.AddHostRouteViaGateway(m.s.ConnectionIP, defIF, gateway, 1); err != nil {
			return fmt.Errorf("add host route via gw: %w", err)
		}
	} else {
		// Rare setups (ICS/bridges) may present empty gw; best-effort on-link via default IF.
		if err := m.netsh.AddHostRouteOnLink(m.s.ConnectionIP, defIF, 1); err != nil {
			return fmt.Errorf("add on-link host route (fallback): %w", err)
		}
	}
	return nil
}

func (m *v6Manager) onLinkInterfaceName(server net.IP) (string, bool) {
	if server == nil || server.To4() != nil {
		return "", false
	}
	iFaces, _ := net.Interfaces()
	for _, it := range iFaces {
		// Prefer UP interfaces (cheap sanity)
		if (it.Flags & net.FlagUp) == 0 {
			continue
		}
		addresses, _ := it.Addrs()
		for _, a := range addresses {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() == nil && ipn.Contains(server) {
				return it.Name, true
			}
		}
	}
	return "", false
}

func (m *v6Manager) assignIPToTunDevice() error {
	ip := net.ParseIP(m.s.InterfaceAddress)
	_, nw, _ := net.ParseCIDR(m.s.InterfaceIPCIDR)
	if ip == nil || ip.To4() != nil || nw == nil || !nw.Contains(ip) {
		_ = m.route.Delete(m.s.ConnectionIP)
		return fmt.Errorf("address %s not in %s", m.s.InterfaceAddress, m.s.InterfaceIPCIDR)
	}
	// For IPv6 netsh expects prefix length as digits (e.g., "64").
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
	// Roll back the /128 host route on failure to avoid crumbs.
	if err := m.netsh.SetDNS(m.s.InterfaceName,
		[]string{"2606:4700:4700::1111", "2001:4860:4860::8888"},
	); err != nil {
		_ = m.route.Delete(m.s.ConnectionIP)
		return err
	}
	_ = m.ipConfig.FlushDNS()
	return nil
}

func (m *v6Manager) DisposeDevices() error {
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
