//go:build windows

package manager

import (
	"fmt"
	"net"
	"strings"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL/windows/ipcfg"
	"tungo/infrastructure/PAL/windows/wtun"
	"tungo/infrastructure/settings"

	"golang.zx2c4.com/wintun"
)

// v4Manager configures a Wintun adapter and the host stack for IPv4.
type v4Manager struct {
	s      settings.Settings
	tun    tun.Device
	netCfg ipcfg.Contract
}

func newV4Manager(
	s settings.Settings,
	netCfg ipcfg.Contract,
) *v4Manager {
	return &v4Manager{
		s:      s,
		netCfg: netCfg,
	}
}

// CreateDevice creates/configures the TUN adapter and system netCfgs/DNS for IPv4.
// Safe order: create adapter → host netCfg to server → assign IP → split default → MTU → DNS.
// On any error after adapter creation we call DisposeDevices() to leave the host clean.
func (m *v4Manager) CreateDevice() (tun.Device, error) {
	if sErr := m.validateSettings(); sErr != nil {
		return nil, sErr
	}
	tunDev, err := m.createOrOpenTunDevice()
	if err != nil {
		return nil, err
	}
	m.tun = tunDev
	if err = m.addStaticRouteToServer(); err != nil {
		_ = m.DisposeDevices()
		return nil, err
	}
	if err = m.assignIPToTunDevice(); err != nil {
		_ = m.DisposeDevices()
		return nil, err
	}
	if err = m.setDefaultRouteToTunDevice(); err != nil {
		_ = m.DisposeDevices()
		return nil, err
	}
	if err = m.setMTUToTunDevice(); err != nil {
		_ = m.DisposeDevices()
		return nil, err
	}
	if err = m.setDNSToTunDevice(); err != nil {
		_ = m.DisposeDevices()
		return nil, err
	}
	return m.tun, nil
}

func (m *v4Manager) validateSettings() error {
	if strings.TrimSpace(m.s.InterfaceName) == "" {
		return fmt.Errorf("empty InterfaceName")
	}
	if net.ParseIP(m.s.Host) == nil {
		return fmt.Errorf("invalid Host: %q", m.s.Host)
	}
	if _, _, err := net.ParseCIDR(m.s.InterfaceSubnet); err != nil {
		return fmt.Errorf("invalid InterfaceSubnet: %q", m.s.InterfaceSubnet)
	}
	if net.ParseIP(m.s.InterfaceIP).To4() == nil {
		return fmt.Errorf("v4Manager requires IPv4 InterfaceIP, got %q", m.s.InterfaceIP)
	}
	if net.ParseIP(m.s.Host).To4() == nil {
		return fmt.Errorf("v4Manager requires IPv4 Host, got %q", m.s.Host)
	}
	return nil
}

// createOrOpenTunDevice creates or opening existing wintun adapter (idempotent behavior).
func (m *v4Manager) createOrOpenTunDevice() (tun.Device, error) {
	adapter, err := wintun.CreateAdapter(m.s.InterfaceName, tunnelType, nil)
	if err != nil {
		if existing, openErr := wintun.OpenAdapter(m.s.InterfaceName); openErr == nil {
			return wtun.NewTUN(existing)
		}
		return nil, fmt.Errorf("create/open adapter: %w", err)
	}
	tunDev, tunDevErr := wtun.NewTUN(adapter)
	if tunDevErr != nil {
		_ = adapter.Close()
		return nil, tunDevErr
	}
	return tunDev, nil
}

func (m *v4Manager) addStaticRouteToServer() error {
	_ = m.netCfg.DeleteRoute(m.s.Host)
	gw, ifName, _, _, err := m.netCfg.BestRoute(m.s.Host)
	if err != nil {
		return err
	}
	if gw == "" {
		// on-link
		return m.netCfg.AddHostRouteOnLink(m.s.Host, ifName, 1)
	}
	return m.netCfg.AddHostRouteViaGateway(m.s.Host, ifName, gw, 1)
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
	ip := net.ParseIP(m.s.InterfaceIP)
	_, network, _ := net.ParseCIDR(m.s.InterfaceSubnet)
	if ip == nil || network == nil || !network.Contains(ip) {
		_ = m.netCfg.DeleteRoute(m.s.Host)
		return fmt.Errorf("address %s not in %s", m.s.InterfaceIP, m.s.InterfaceSubnet)
	}
	mask := net.IP(network.Mask).String() // dotted decimal mask
	if err := m.netCfg.SetAddressStatic(m.s.InterfaceName, m.s.InterfaceIP, mask); err != nil {
		_ = m.netCfg.DeleteRoute(m.s.Host)
		return err
	}
	return nil
}

// setDefaultRouteToTunDevice replaces any existing default route with split default route (0.0.0.0/1, 128.0.0.0/1).
func (m *v4Manager) setDefaultRouteToTunDevice() error {
	_ = m.netCfg.DeleteDefaultSplitRoutes(m.s.InterfaceName)
	if err := m.netCfg.AddDefaultSplitRoutes(m.s.InterfaceName, 1); err != nil {
		_ = m.netCfg.DeleteRoute(m.s.Host)
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
	if err := m.netCfg.SetMTU(m.s.InterfaceName, mtu); err != nil {
		_ = m.netCfg.DeleteRoute(m.s.Host)
		return err
	}
	return nil
}

// setDNSToTunDevice applies v4 DNS resolvers and flushes system cache.
func (m *v4Manager) setDNSToTunDevice() error {
	//ToDo: move dns server addresses to configuration
	if err := m.netCfg.SetDNS(m.s.InterfaceName, []string{"1.1.1.1", "8.8.8.8"}); err != nil {
		return err
	}
	_ = m.netCfg.FlushDNS()
	return nil
}

// DisposeDevices reverses CreateDevice in safe order.
func (m *v4Manager) DisposeDevices() error {
	_ = m.netCfg.DeleteDefaultSplitRoutes(m.s.InterfaceName)
	_ = m.netCfg.DeleteRoute(m.s.Host)
	if m.tun != nil {
		_ = m.tun.Close()
	}
	return nil
}
