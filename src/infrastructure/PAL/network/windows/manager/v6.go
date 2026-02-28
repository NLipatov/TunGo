//go:build windows

package manager

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL/network/windows/ipcfg"
	"tungo/infrastructure/PAL/network/windows/wtun"
	"tungo/infrastructure/settings"

	"golang.zx2c4.com/wintun"
)

// v6Manager configures a Wintun adapter and the host stack for IPv6.
type v6Manager struct {
	s                  settings.Settings
	tun                tun.Device
	netConfig          ipcfg.Contract
	routeEndpoint      netip.AddrPort
	createTunDeviceFn  func() (tun.Device, error)
	resolveRouteIPv6Fn func() (string, error)
	resolvedRouteIP    string // cached resolved server IP for consistent teardown
	resolvedRouteIf    string // cached egress interface used for host route
}

func newV6Manager(
	s settings.Settings,
	netConfig ipcfg.Contract,
) *v6Manager {
	return &v6Manager{
		s:         s,
		netConfig: netConfig,
	}
}

// CreateDevice creates/configures the TUN adapter and system routes/DNS for IPv6.
// Safe order mirrors v4 with IPv6-specific details.
func (m *v6Manager) CreateDevice() (tun.Device, error) {
	if err := m.validateSettings(); err != nil {
		return nil, err
	}

	createTun := m.createTunDeviceFn
	if createTun == nil {
		createTun = m.createTunDevice
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
	if err = m.setRouteToTunDevice(); err != nil {
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

func (m *v6Manager) validateSettings() error {
	if strings.TrimSpace(m.s.TunName) == "" {
		return fmt.Errorf("empty TunName")
	}
	if m.s.Server.IsZero() {
		return fmt.Errorf("empty Server")
	}
	if !m.s.IPv6Subnet.IsValid() {
		return fmt.Errorf("invalid IPv6Subnet: %q", m.s.IPv6Subnet)
	}
	if !m.s.IPv6.IsValid() || m.s.IPv6.Unmap().Is4() {
		return fmt.Errorf("v6Manager requires IPv6 address, got %q", m.s.IPv6)
	}
	return nil
}

func (m *v6Manager) createTunDevice() (tun.Device, error) {
	adapter, err := wintun.CreateAdapter(m.s.TunName, tunnelType, nil)
	if err != nil {
		if existing, openErr := wintun.OpenAdapter(m.s.TunName); openErr == nil {
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
	routeIP, err := m.resolveRouteIPv6()
	if err != nil {
		// If control channel is pinned to IPv4 endpoint, there is no IPv6 host route to preserve.
		if m.routeEndpoint.IsValid() && m.routeEndpoint.Addr().Unmap().Is4() {
			return nil
		}
		return fmt.Errorf("resolve host %s: %w", m.s.Server, err)
	}
	gw, ifName, ifIndex, _, bestErr := m.netConfig.BestRoute(routeIP)
	if bestErr != nil {
		return bestErr
	}
	ifName, err = routeInterfaceName(ifName, ifIndex)
	if err != nil {
		return err
	}
	m.resolvedRouteIP = routeIP
	m.resolvedRouteIf = ifName
	_ = m.netConfig.DeleteRoute(routeIP)
	_ = m.netConfig.DeleteRouteOnInterface(routeIP, ifName)
	if gw == "" {
		// on-link
		return m.netConfig.AddHostRouteOnLink(routeIP, ifName, 1)
	}
	return m.netConfig.AddHostRouteViaGateway(routeIP, ifName, gw, 1)
}

// assignIPToTunDevice validates IPv6 address ∈ CIDR and applies it via prefix length.
func (m *v6Manager) assignIPToTunDevice() error {
	ipStr := m.s.IPv6.String()
	subnetStr := m.s.IPv6Subnet.String()
	ip := net.ParseIP(ipStr)
	_, nw, _ := net.ParseCIDR(subnetStr)
	if ip == nil || ip.To4() != nil || nw == nil || !nw.Contains(ip) {
		return fmt.Errorf("address %s not in %s", ipStr, subnetStr)
	}
	prefix, _ := nw.Mask.Size()
	return m.netConfig.SetAddressStatic(m.s.TunName, ipStr, strconv.Itoa(prefix))
}

// setRouteToTunDevice replaces any existing default with IPv6 split default (::/1, 8000::/1).
func (m *v6Manager) setRouteToTunDevice() error {
	_ = m.netConfig.DeleteDefaultSplitRoutes(m.s.TunName)
	return m.netConfig.AddDefaultSplitRoutes(m.s.TunName, 1)
}

func (m *v6Manager) setMTUToTunDevice() error {
	mtu := m.s.MTU
	if mtu == 0 {
		mtu = settings.SafeMTU
	}
	if mtu < settings.MinimumIPv6MTU {
		mtu = settings.MinimumIPv6MTU
	}
	return m.netConfig.SetMTU(m.s.TunName, mtu)
}

func (m *v6Manager) setDNSToTunDevice() error {
	if err := m.netConfig.SetDNS(m.s.TunName, m.s.DNSv6Resolvers()); err != nil {
		return err
	}
	if err := m.netConfig.FlushDNS(); err != nil {
		log.Printf("failed to flush IPv6 DNS cache: %v", err)
	}
	return nil
}

// DisposeDevices reverses CreateDevice in safe order.
func (m *v6Manager) DisposeDevices() error {
	var cleanupErrs []error
	if err := m.netConfig.DeleteDefaultSplitRoutes(m.s.TunName); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("delete default split routes: %w", err))
	}
	if m.resolvedRouteIP != "" {
		if err := m.netConfig.DeleteRouteOnInterface(m.resolvedRouteIP, m.resolvedRouteIf); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("delete route %s on %s: %w", m.resolvedRouteIP, m.resolvedRouteIf, err))
		}
	}
	if err := m.netConfig.SetDNS(m.s.TunName, nil); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("clear DNS: %w", err))
	}
	if err := m.netConfig.FlushDNS(); err != nil {
		log.Printf("failed to flush IPv6 DNS cache during cleanup: %v", err)
	}
	if m.tun != nil {
		if err := m.tun.Close(); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("close tun: %w", err))
		}
	}
	return errors.Join(cleanupErrs...)
}

func (m *v6Manager) SetRouteEndpoint(addr netip.AddrPort) {
	m.routeEndpoint = addr
}

func (m *v6Manager) resolveRouteIPv6() (string, error) {
	if m.routeEndpoint.IsValid() {
		ip := m.routeEndpoint.Addr()
		if !ip.Unmap().Is4() {
			return ip.String(), nil
		}
		return "", fmt.Errorf("route endpoint %s is IPv4, expected IPv6", ip)
	}
	if m.resolveRouteIPv6Fn != nil {
		return m.resolveRouteIPv6Fn()
	}
	ctx, cancel := context.WithTimeout(context.Background(), routeResolveTimeout(m.s))
	defer cancel()
	return m.s.Server.RouteIPv6Context(ctx)
}
