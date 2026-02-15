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
	s               settings.Settings
	tun             tun.Device
	netCfg          ipcfg.Contract
	resolvedRouteIP string // cached resolved server IP for consistent teardown
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
	if m.s.Host.IsZero() {
		return fmt.Errorf("empty Host")
	}
	if !m.s.IPv4Subnet.IsValid() {
		return fmt.Errorf("invalid IPv4Subnet: %q", m.s.IPv4Subnet)
	}
	if !m.s.IPv4IP.IsValid() || !m.s.IPv4IP.Unmap().Is4() {
		return fmt.Errorf("v4Manager requires IPv4 IPv4IP, got %q", m.s.IPv4IP)
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
	routeIP, err := m.s.Host.RouteIPv4()
	if err != nil {
		return fmt.Errorf("resolve host %s: %w", m.s.Host, err)
	}
	m.resolvedRouteIP = routeIP
	_ = m.netCfg.DeleteRoute(routeIP)
	gw, ifName, _, _, bestErr := m.netCfg.BestRoute(routeIP)
	if bestErr != nil {
		return bestErr
	}
	if gw == "" {
		// on-link
		return m.netCfg.AddHostRouteOnLink(routeIP, ifName, 1)
	}
	return m.netCfg.AddHostRouteViaGateway(routeIP, ifName, gw, 1)
}

// assignIPToTunDevice validates IPv4 address ∈ CIDR and applies it.
func (m *v4Manager) assignIPToTunDevice() error {
	ipStr := m.s.IPv4IP.String()
	subnetStr := m.s.IPv4Subnet.String()
	ip := net.ParseIP(ipStr)
	_, network, _ := net.ParseCIDR(subnetStr)
	if ip == nil || network == nil || !network.Contains(ip) {
		return fmt.Errorf("address %s not in %s", ipStr, subnetStr)
	}
	mask := net.IP(network.Mask).String() // dotted decimal mask
	if err := m.netCfg.SetAddressStatic(m.s.InterfaceName, ipStr, mask); err != nil {
		return err
	}
	return nil
}

// setDefaultRouteToTunDevice replaces any existing default route with split default route (0.0.0.0/1, 128.0.0.0/1).
func (m *v4Manager) setDefaultRouteToTunDevice() error {
	_ = m.netCfg.DeleteDefaultSplitRoutes(m.s.InterfaceName)
	return m.netCfg.AddDefaultSplitRoutes(m.s.InterfaceName, 1)
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
	return m.netCfg.SetMTU(m.s.InterfaceName, mtu)
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
	if m.resolvedRouteIP != "" {
		_ = m.netCfg.DeleteRoute(m.resolvedRouteIP)
	}
	if m.tun != nil {
		_ = m.tun.Close()
	}
	return nil
}
