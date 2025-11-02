//go:build windows

package manager

import (
	"fmt"
	"net"
	"strconv"
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
	if strings.TrimSpace(m.s.InterfaceName) == "" {
		return nil, fmt.Errorf("empty InterfaceName")
	}
	if net.ParseIP(m.s.ConnectionIP) == nil {
		return nil, fmt.Errorf("invalid ConnectionIP: %q", m.s.ConnectionIP)
	}
	if _, _, err := net.ParseCIDR(m.s.InterfaceIPCIDR); err != nil {
		return nil, fmt.Errorf("invalid InterfaceIPCIDR: %q", m.s.InterfaceIPCIDR)
	}
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

func (m *v6Manager) addStaticRouteToServer() error {
	_ = m.route.Delete(m.s.ConnectionIP)
	gw, ifName, _, _, err := m.route.BestRoute(m.s.ConnectionIP)
	if err != nil {
		return err
	}
	if gw == "" {
		// on-link
		return m.netsh.AddHostRouteOnLink(m.s.ConnectionIP, ifName, 1)
	}
	return m.netsh.AddHostRouteViaGateway(m.s.ConnectionIP, ifName, gw, 1)
}

// onLinkInterfaceName returns the name of an interface whose IPv6 prefix contains 'server'.
func (m *v6Manager) onLinkInterfaceName(server net.IP) (string, bool) {
	if server == nil || server.To4() != nil {
		return "", false
	}
	iFaces, _ := net.Interfaces()
	for _, iFace := range iFaces {
		if !m.isCandidateIF(iFace, m.s.InterfaceName) {
			continue
		}
		addresses, _ := iFace.Addrs()
		for _, a := range addresses {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() == nil && ipn.Contains(server) {
				return iFace.Name, true
			}
		}
	}
	return "", false
}

func (m *v6Manager) isCandidateIF(it net.Interface, selfName string) bool {
	// Only UP, non-loopback, and not our own TUN
	if (it.Flags & net.FlagUp) == 0 {
		return false
	}
	if (it.Flags & net.FlagLoopback) != 0 {
		return false
	}
	if it.Name == selfName {
		return false
	}
	return true
}

// assignIPToTunDevice validates IPv6 address ∈ CIDR and applies it via prefix length.
func (m *v6Manager) assignIPToTunDevice() error {
	ip := net.ParseIP(m.s.InterfaceAddress)
	_, nw, _ := net.ParseCIDR(m.s.InterfaceIPCIDR)
	if ip == nil || ip.To4() != nil || nw == nil || !nw.Contains(ip) {
		_ = m.route.Delete(m.s.ConnectionIP)
		return fmt.Errorf("address %s not in %s", m.s.InterfaceAddress, m.s.InterfaceIPCIDR)
	}
	prefix, _ := nw.Mask.Size()
	if err := m.netsh.SetAddressStatic(m.s.InterfaceName, m.s.InterfaceAddress, strconv.Itoa(prefix)); err != nil {
		_ = m.route.Delete(m.s.ConnectionIP)
		return err
	}
	return nil
}

// setRouteToTunDevice replaces any existing default with IPv6 split default (::/1, 8000::/1).
func (m *v6Manager) setRouteToTunDevice() error {
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
	if mtu < settings.MinimumIPv6MTU {
		mtu = settings.MinimumIPv6MTU
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
