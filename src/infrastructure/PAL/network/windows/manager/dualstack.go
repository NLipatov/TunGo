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

// dualStackManager configures one Wintun adapter for both IPv4 and IPv6 stacks.
type dualStackManager struct {
	s   settings.Settings
	tun tun.Device

	netCfg4 ipcfg.Contract
	netCfg6 ipcfg.Contract

	// test hooks
	createTunDeviceFn  func() (tun.Device, error)
	resolveRouteIPv4Fn func() (string, error)
	resolveRouteIPv6Fn func() (string, error)
	routeEndpoint      netip.AddrPort
	resolvedRouteIP4   string // cached resolved server IPv4 for teardown
	resolvedRouteIP6   string // cached resolved server IPv6 for teardown
	resolvedRouteIf4   string // cached egress interface for IPv4 host route
	resolvedRouteIf6   string // cached egress interface for IPv6 host route
}

func newDualStackManager(
	s settings.Settings,
	netCfg4, netCfg6 ipcfg.Contract,
) *dualStackManager {
	return &dualStackManager{
		s:       s,
		netCfg4: netCfg4,
		netCfg6: netCfg6,
	}
}

func (m *dualStackManager) CreateDevice() (tun.Device, error) {
	if err := m.validateSettings(); err != nil {
		return nil, err
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

	if err = m.addStaticRouteToServer4(); err != nil {
		_ = m.DisposeDevices()
		return nil, err
	}
	if err = m.addStaticRouteToServer6(); err != nil {
		_ = m.DisposeDevices()
		return nil, err
	}
	if err = m.assignIPv4ToTunDevice(); err != nil {
		_ = m.DisposeDevices()
		return nil, err
	}
	if err = m.assignIPv6ToTunDevice(); err != nil {
		_ = m.DisposeDevices()
		return nil, err
	}
	if err = m.setDefaultRoutesToTunDevice(); err != nil {
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

func (m *dualStackManager) validateSettings() error {
	if strings.TrimSpace(m.s.TunName) == "" {
		return fmt.Errorf("empty TunName")
	}
	if m.s.Server.IsZero() {
		return fmt.Errorf("empty Server")
	}
	if !m.s.IPv4Subnet.IsValid() {
		return fmt.Errorf("invalid IPv4Subnet: %q", m.s.IPv4Subnet)
	}
	if !m.s.IPv6Subnet.IsValid() {
		return fmt.Errorf("invalid IPv6Subnet: %q", m.s.IPv6Subnet)
	}
	if !m.s.IPv4.IsValid() || !m.s.IPv4.Unmap().Is4() {
		return fmt.Errorf("dualStackManager requires valid IPv4, got %q", m.s.IPv4)
	}
	if !m.s.IPv6.IsValid() || m.s.IPv6.Unmap().Is4() {
		return fmt.Errorf("dualStackManager requires valid IPv6, got %q", m.s.IPv6)
	}
	return nil
}

func (m *dualStackManager) createOrOpenTunDevice() (tun.Device, error) {
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

func (m *dualStackManager) addStaticRouteToServer4() error {
	if m.routeEndpoint.IsValid() && !m.routeEndpoint.Addr().Unmap().Is4() {
		return nil
	}

	routeIP, err := m.resolveRouteIPv4()
	if err != nil {
		if m.shouldSkipIPv4RouteOnResolveError() {
			return nil
		}
		return fmt.Errorf("resolve IPv4 host %s: %w", m.s.Server, err)
	}
	gw, ifName, ifIndex, _, bestErr := m.netCfg4.BestRoute(routeIP)
	if bestErr != nil {
		return bestErr
	}
	ifName, err = routeInterfaceName(ifName, ifIndex)
	if err != nil {
		return err
	}
	m.resolvedRouteIP4 = routeIP
	m.resolvedRouteIf4 = ifName
	_ = m.netCfg4.DeleteRoute(routeIP)
	_ = m.netCfg4.DeleteRouteOnInterface(routeIP, ifName)
	if gw == "" {
		return m.netCfg4.AddHostRouteOnLink(routeIP, ifName, 1)
	}
	return m.netCfg4.AddHostRouteViaGateway(routeIP, ifName, gw, 1)
}

func (m *dualStackManager) addStaticRouteToServer6() error {
	if m.routeEndpoint.IsValid() && m.routeEndpoint.Addr().Unmap().Is4() {
		return nil
	}

	routeIP, err := m.resolveRouteIPv6()
	if err != nil {
		if m.shouldSkipIPv6RouteOnResolveError() {
			return nil
		}
		return fmt.Errorf("resolve IPv6 host %s: %w", m.s.Server, err)
	}
	gw, ifName, ifIndex, _, bestErr := m.netCfg6.BestRoute(routeIP)
	if bestErr != nil {
		return bestErr
	}
	ifName, err = routeInterfaceName(ifName, ifIndex)
	if err != nil {
		return err
	}
	m.resolvedRouteIP6 = routeIP
	m.resolvedRouteIf6 = ifName
	_ = m.netCfg6.DeleteRoute(routeIP)
	_ = m.netCfg6.DeleteRouteOnInterface(routeIP, ifName)
	if gw == "" {
		return m.netCfg6.AddHostRouteOnLink(routeIP, ifName, 1)
	}
	return m.netCfg6.AddHostRouteViaGateway(routeIP, ifName, gw, 1)
}

func (m *dualStackManager) assignIPv4ToTunDevice() error {
	ipStr := m.s.IPv4.String()
	subnetStr := m.s.IPv4Subnet.String()
	ip := net.ParseIP(ipStr)
	_, nw, _ := net.ParseCIDR(subnetStr)
	if ip == nil || nw == nil || !nw.Contains(ip) {
		return fmt.Errorf("address %s not in %s", ipStr, subnetStr)
	}
	mask := net.IP(nw.Mask).String()
	return m.netCfg4.SetAddressStatic(m.s.TunName, ipStr, mask)
}

func (m *dualStackManager) assignIPv6ToTunDevice() error {
	ipStr := m.s.IPv6.String()
	subnetStr := m.s.IPv6Subnet.String()
	ip := net.ParseIP(ipStr)
	_, nw, _ := net.ParseCIDR(subnetStr)
	if ip == nil || ip.To4() != nil || nw == nil || !nw.Contains(ip) {
		return fmt.Errorf("address %s not in %s", ipStr, subnetStr)
	}
	prefix, _ := nw.Mask.Size()
	return m.netCfg6.SetAddressStatic(m.s.TunName, ipStr, strconv.Itoa(prefix))
}

func (m *dualStackManager) setDefaultRoutesToTunDevice() error {
	_ = m.netCfg4.DeleteDefaultSplitRoutes(m.s.TunName)
	if err := m.netCfg4.AddDefaultSplitRoutes(m.s.TunName, 1); err != nil {
		return err
	}

	_ = m.netCfg6.DeleteDefaultSplitRoutes(m.s.TunName)
	return m.netCfg6.AddDefaultSplitRoutes(m.s.TunName, 1)
}

func (m *dualStackManager) setMTUToTunDevice() error {
	mtu := m.s.MTU
	if mtu == 0 {
		mtu = settings.SafeMTU
	}
	if mtu < settings.MinimumIPv6MTU {
		mtu = settings.MinimumIPv6MTU
	}

	if err := m.netCfg4.SetMTU(m.s.TunName, mtu); err != nil {
		return fmt.Errorf("set IPv4 MTU: %w", err)
	}
	if err := m.netCfg6.SetMTU(m.s.TunName, mtu); err != nil {
		return fmt.Errorf("set IPv6 MTU: %w", err)
	}
	return nil
}

func (m *dualStackManager) setDNSToTunDevice() error {
	if err := m.netCfg4.SetDNS(m.s.TunName, m.s.DNSv4Resolvers()); err != nil {
		return err
	}
	if err := m.netCfg6.SetDNS(m.s.TunName, m.s.DNSv6Resolvers()); err != nil {
		return err
	}
	if err := m.netCfg4.FlushDNS(); err != nil {
		log.Printf("failed to flush IPv4 DNS cache: %v", err)
	}
	if err := m.netCfg6.FlushDNS(); err != nil {
		log.Printf("failed to flush IPv6 DNS cache: %v", err)
	}
	return nil
}

func (m *dualStackManager) DisposeDevices() error {
	var cleanupErrs []error
	if err := m.netCfg4.DeleteDefaultSplitRoutes(m.s.TunName); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("delete IPv4 split routes: %w", err))
	}
	if err := m.netCfg6.DeleteDefaultSplitRoutes(m.s.TunName); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("delete IPv6 split routes: %w", err))
	}

	if m.resolvedRouteIP4 != "" {
		if err := m.netCfg4.DeleteRouteOnInterface(m.resolvedRouteIP4, m.resolvedRouteIf4); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("delete IPv4 route %s on %s: %w", m.resolvedRouteIP4, m.resolvedRouteIf4, err))
		}
	}
	if m.resolvedRouteIP6 != "" {
		if err := m.netCfg6.DeleteRouteOnInterface(m.resolvedRouteIP6, m.resolvedRouteIf6); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("delete IPv6 route %s on %s: %w", m.resolvedRouteIP6, m.resolvedRouteIf6, err))
		}
	}
	// Best-effort DNS cleanup to avoid leaving partial resolver state on rollback.
	if err := m.netCfg4.SetDNS(m.s.TunName, nil); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("clear IPv4 DNS: %w", err))
	}
	if err := m.netCfg6.SetDNS(m.s.TunName, nil); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("clear IPv6 DNS: %w", err))
	}
	if err := m.netCfg4.FlushDNS(); err != nil {
		log.Printf("failed to flush IPv4 DNS cache during cleanup: %v", err)
	}
	if err := m.netCfg6.FlushDNS(); err != nil {
		log.Printf("failed to flush IPv6 DNS cache during cleanup: %v", err)
	}

	if m.tun != nil {
		if err := m.tun.Close(); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("close tun: %w", err))
		}
	}
	return errors.Join(cleanupErrs...)
}

func (m *dualStackManager) resolveRouteIPv4() (string, error) {
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

func (m *dualStackManager) resolveRouteIPv6() (string, error) {
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

func (m *dualStackManager) SetRouteEndpoint(addr netip.AddrPort) {
	m.routeEndpoint = addr
}

func (m *dualStackManager) shouldSkipIPv4RouteOnResolveError() bool {
	if _, isDomain := m.s.Server.Domain(); !isDomain {
		return m.s.Server.HasIPv6() && !m.s.Server.HasIPv4()
	}
	_, err := m.resolveRouteIPv6()
	return err == nil
}

func (m *dualStackManager) shouldSkipIPv6RouteOnResolveError() bool {
	if _, isDomain := m.s.Server.Domain(); !isDomain {
		return m.s.Server.HasIPv4() && !m.s.Server.HasIPv6()
	}
	_, err := m.resolveRouteIPv4()
	return err == nil
}
