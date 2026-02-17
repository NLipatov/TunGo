//go:build windows

package manager

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"strings"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL/windows/ipcfg"
	"tungo/infrastructure/PAL/windows/wtun"
	"tungo/infrastructure/settings"

	"golang.zx2c4.com/wintun"
)

// v4Manager configures a Wintun adapter and the host stack for IPv4.
type v4Manager struct {
	s                  settings.Settings
	tun                tun.Device
	netCfg             ipcfg.Contract
	routeEndpoint      netip.AddrPort
	createTunDeviceFn  func() (tun.Device, error)
	resolveRouteIPv4Fn func() (string, error)
	resolvedRouteIP    string // cached resolved server IP for consistent teardown
	resolvedRouteIf    string // cached egress interface used for host route
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
	createTun := m.createTunDeviceFn
	if createTun == nil {
		createTun = m.createOrOpenTunDevice
	}
	tunDev, err := createTun()
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
	if strings.TrimSpace(m.s.TunName) == "" {
		return fmt.Errorf("empty TunName")
	}
	if m.s.Server.IsZero() {
		return fmt.Errorf("empty Server")
	}
	if !m.s.IPv4Subnet.IsValid() {
		return fmt.Errorf("invalid IPv4Subnet: %q", m.s.IPv4Subnet)
	}
	if !m.s.IPv4.IsValid() || !m.s.IPv4.Unmap().Is4() {
		return fmt.Errorf("v4Manager requires valid IPv4, got %q", m.s.IPv4)
	}
	return nil
}

// createOrOpenTunDevice creates or opening existing wintun adapter (idempotent behavior).
func (m *v4Manager) createOrOpenTunDevice() (tun.Device, error) {
	adapter, err := wintun.CreateAdapter(m.s.TunName, tunnelType, nil)
	if err != nil {
		if existing, openErr := wintun.OpenAdapter(m.s.TunName); openErr == nil {
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
	routeIP, err := m.resolveRouteIPv4()
	if err != nil {
		// If control channel is pinned to IPv6 endpoint, there is no IPv4 host route to preserve.
		if m.routeEndpoint.IsValid() && !m.routeEndpoint.Addr().Unmap().Is4() {
			return nil
		}
		return fmt.Errorf("resolve host %s: %w", m.s.Server, err)
	}
	gw, ifName, ifIndex, _, bestErr := m.netCfg.BestRoute(routeIP)
	if bestErr != nil {
		return bestErr
	}
	ifName, err = routeInterfaceName(ifName, ifIndex)
	if err != nil {
		return err
	}
	m.resolvedRouteIP = routeIP
	m.resolvedRouteIf = ifName
	_ = m.netCfg.DeleteRoute(routeIP)
	_ = m.netCfg.DeleteRouteOnInterface(routeIP, ifName)
	if gw == "" {
		// on-link
		return m.netCfg.AddHostRouteOnLink(routeIP, ifName, 1)
	}
	return m.netCfg.AddHostRouteViaGateway(routeIP, ifName, gw, 1)
}

// assignIPToTunDevice validates IPv4 address ∈ CIDR and applies it.
func (m *v4Manager) assignIPToTunDevice() error {
	ipStr := m.s.IPv4.String()
	subnetStr := m.s.IPv4Subnet.String()
	ip := net.ParseIP(ipStr)
	_, network, _ := net.ParseCIDR(subnetStr)
	if ip == nil || network == nil || !network.Contains(ip) {
		return fmt.Errorf("address %s not in %s", ipStr, subnetStr)
	}
	mask := net.IP(network.Mask).String() // dotted decimal mask
	if err := m.netCfg.SetAddressStatic(m.s.TunName, ipStr, mask); err != nil {
		return err
	}
	return nil
}

// setDefaultRouteToTunDevice replaces any existing default route with split default route (0.0.0.0/1, 128.0.0.0/1).
func (m *v4Manager) setDefaultRouteToTunDevice() error {
	_ = m.netCfg.DeleteDefaultSplitRoutes(m.s.TunName)
	return m.netCfg.AddDefaultSplitRoutes(m.s.TunName, 1)
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
	return m.netCfg.SetMTU(m.s.TunName, mtu)
}

// setDNSToTunDevice applies v4 DNS resolvers and flushes system cache.
func (m *v4Manager) setDNSToTunDevice() error {
	if err := m.netCfg.SetDNS(m.s.TunName, m.s.DNSv4Resolvers()); err != nil {
		return err
	}
	if err := m.netCfg.FlushDNS(); err != nil {
		log.Printf("failed to flush IPv4 DNS cache: %v", err)
	}
	return nil
}

// DisposeDevices reverses CreateDevice in safe order.
func (m *v4Manager) DisposeDevices() error {
	var cleanupErrs []error
	if err := m.netCfg.DeleteDefaultSplitRoutes(m.s.TunName); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("delete default split routes: %w", err))
	}
	if m.resolvedRouteIP != "" {
		if err := m.netCfg.DeleteRouteOnInterface(m.resolvedRouteIP, m.resolvedRouteIf); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("delete route %s on %s: %w", m.resolvedRouteIP, m.resolvedRouteIf, err))
		}
	}
	if err := m.netCfg.SetDNS(m.s.TunName, nil); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("clear DNS: %w", err))
	}
	if err := m.netCfg.FlushDNS(); err != nil {
		log.Printf("failed to flush IPv4 DNS cache during cleanup: %v", err)
	}
	if m.tun != nil {
		if err := m.tun.Close(); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("close tun: %w", err))
		}
	}
	return errors.Join(cleanupErrs...)
}

func (m *v4Manager) SetRouteEndpoint(addr netip.AddrPort) {
	m.routeEndpoint = addr
}

func (m *v4Manager) resolveRouteIPv4() (string, error) {
	if m.routeEndpoint.IsValid() {
		ip := m.routeEndpoint.Addr()
		if ip.Unmap().Is4() {
			return ip.Unmap().String(), nil
		}
		return "", fmt.Errorf("route endpoint %s is IPv6, expected IPv4", ip)
	}
	if m.resolveRouteIPv4Fn != nil {
		return m.resolveRouteIPv4Fn()
	}
	ctx, cancel := context.WithTimeout(context.Background(), routeResolveTimeout(m.s))
	defer cancel()
	return m.s.Server.RouteIPv4Context(ctx)
}
