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

// v4Manager configures a Wintun adapter and the host stack for IPv4.
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

// CreateDevice creates/configures the TUN adapter and system routes/DNS for IPv4.
// Safe order: create adapter → host route to server → assign IP → split default → MTU → DNS.
// On any error after adapter creation we call DisposeDevices() to leave the host clean.
func (m *v4Manager) CreateDevice() (tun.Device, error) {
	if strings.TrimSpace(m.s.InterfaceName) == "" {
		return nil, fmt.Errorf("empty InterfaceName")
	}
	if net.ParseIP(m.s.ConnectionIP) == nil {
		return nil, fmt.Errorf("invalid ConnectionIP: %q", m.s.ConnectionIP)
	}
	if _, _, err := net.ParseCIDR(m.s.InterfaceIPCIDR); err != nil {
		return nil, fmt.Errorf("invalid InterfaceIPCIDR: %q", m.s.InterfaceIPCIDR)
	}
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

func (m *v4Manager) createTunDevice() (tun.Device, error) {
	// Create new; if name already taken, try opening existing (idempotent behavior).
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

func (m *v4Manager) addStaticRouteToServer() error {
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

// onLinkInterfaceName returns the name of an interface whose IPv4 prefix contains 'server'.
func (m *v4Manager) onLinkInterfaceName(server net.IP) (string, bool) {
	srv4 := server.To4()
	if srv4 == nil {
		return "", false
	}
	iFaces, _ := net.Interfaces()
	for _, iFace := range iFaces {
		if !m.isCandidateIF(iFace, m.s.InterfaceName) {
			continue
		}
		addresses, _ := iFace.Addrs()
		for _, a := range addresses {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil && ipn.Contains(srv4) {
				return iFace.Name, true
			}
		}
	}
	return "", false
}

func (m *v4Manager) isCandidateIF(it net.Interface, selfName string) bool {
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

// assignIPToTunDevice validates IPv4 address ∈ CIDR and applies it.
func (m *v4Manager) assignIPToTunDevice() error {
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

// setRouteToTunDevice replaces any existing default with split default (0.0.0.0/1, 128.0.0.0/1).
func (m *v4Manager) setRouteToTunDevice() error {
	_ = m.netsh.DeleteDefaultSplitRoutes(m.s.InterfaceName)
	if err := m.netsh.AddDefaultSplitRoutes(m.s.InterfaceName, 1); err != nil {
		_ = m.route.Delete(m.s.ConnectionIP)
		return err
	}
	return nil
}

// setMTUToTunDevice sets MTU (or safe default).
func (m *v4Manager) setMTUToTunDevice() error {
	mtu := m.s.MTU
	if mtu == 0 {
		mtu = settings.SafeMTU
	}
	if mtu < settings.MinimumIPv4MTU {
		mtu = settings.MinimumIPv4MTU
	}
	if err := m.netsh.SetMTU(m.s.InterfaceName, mtu); err != nil {
		_ = m.route.Delete(m.s.ConnectionIP)
		return err
	}
	return nil
}

// setDNSToTunDevice applies v4 DNS resolvers and flushes system cache.
func (m *v4Manager) setDNSToTunDevice() error {
	if err := m.netsh.SetDNS(m.s.InterfaceName, []string{"1.1.1.1", "8.8.8.8"}); err != nil {
		return err
	}
	_ = m.ipConfig.FlushDNS()
	return nil
}

// DisposeDevices reverses CreateDevice in safe order.
func (m *v4Manager) DisposeDevices() error {
	_ = m.route.Delete(m.s.ConnectionIP)
	if m.tun != nil {
		_ = m.tun.Close()
	}
	return nil
}
