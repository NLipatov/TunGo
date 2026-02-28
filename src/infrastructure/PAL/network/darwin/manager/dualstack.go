//go:build darwin

package manager

import (
	"fmt"
	"net/netip"
	"strings"

	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL/network/darwin/ifconfig"
	"tungo/infrastructure/PAL/network/darwin/route"
	"tungo/infrastructure/PAL/network/darwin/utun"
	"tungo/infrastructure/settings"
)

// dualStack manages a single utun device with both IPv4 and IPv6 addresses and routes.
// macOS utun natively supports dual-stack via its AF header — no need for two devices.
type dualStack struct {
	s                settings.Settings
	tunDev           tun.Device
	rawUTUN          utun.UTUN
	ifc4             ifconfig.Contract
	ifc6             ifconfig.Contract
	rtc4             route.Contract
	rtc6             route.Contract
	ifName           string
	routeEndpoint    netip.AddrPort
	resolvedRouteIP4 string // cached resolved IPv4 server IP for consistent teardown
	resolvedRouteIP6 string // cached resolved IPv6 server IP for consistent teardown
}

func (m *dualStack) SetRouteEndpoint(addr netip.AddrPort) {
	m.routeEndpoint = addr
}

func newDualStack(
	s settings.Settings,
	ifc4 ifconfig.Contract,
	ifc6 ifconfig.Contract,
	rtc4 route.Contract,
	rtc6 route.Contract,
) *dualStack {
	return &dualStack{
		s:    s,
		ifc4: ifc4,
		ifc6: ifc6,
		rtc4: rtc4,
		rtc6: rtc6,
	}
}

func (m *dualStack) CreateDevice() (tun.Device, error) {
	if err := m.validateSettings(); err != nil {
		return nil, err
	}

	raw, err := utun.NewDefaultFactory(m.ifc4).CreateTUN(m.effectiveMTU())
	if err != nil {
		return nil, fmt.Errorf("create utun: %w", err)
	}
	m.rawUTUN = raw

	name, err := raw.Name()
	if err != nil {
		_ = m.DisposeDevices()
		return nil, fmt.Errorf("get utun name: %w", err)
	}
	m.ifName = name

	// Pin route to IPv4 server.
	routeIP4, route4Err := m.resolveRouteIPv4()
	if route4Err == nil {
		m.resolvedRouteIP4 = routeIP4
		if err := m.rtc4.Get(routeIP4); err != nil {
			_ = m.DisposeDevices()
			return nil, fmt.Errorf("dualstack: pin v4 route to %s: %w", m.s.Server, err)
		}
	} else if !shouldSkipDarwinIPv4Route(route4Err) {
		_ = m.DisposeDevices()
		return nil, fmt.Errorf("dualstack: resolve v4 route for %s: %w", m.s.Server, route4Err)
	}

	// Pin route to IPv6 server (if configured).
	routeIP6, route6Err := m.resolveRouteIPv6()
	if route6Err == nil {
		m.resolvedRouteIP6 = routeIP6
		if err := m.rtc6.Get(routeIP6); err != nil {
			_ = m.DisposeDevices()
			return nil, fmt.Errorf("dualstack: pin v6 route to %s: %w", m.s.Server, err)
		}
	} else if !shouldSkipDarwinIPv6Route(route6Err) {
		_ = m.DisposeDevices()
		return nil, fmt.Errorf("dualstack: resolve v6 route for %s: %w", m.s.Server, route6Err)
	}

	// Assign IPv4 address.
	cidr4 := fmt.Sprintf("%s/32", m.s.IPv4)
	if err := m.ifc4.LinkAddrAdd(m.ifName, cidr4); err != nil {
		_ = m.DisposeDevices()
		return nil, fmt.Errorf("dualstack: set v4 addr %s on %s: %w", cidr4, m.ifName, err)
	}

	// Assign IPv6 address.
	cidr6, _ := m.s.IPv6CIDR()
	if cidr6 == "" {
		cidr6 = fmt.Sprintf("%s/128", m.s.IPv6)
	}
	if err := m.ifc6.LinkAddrAdd(m.ifName, cidr6); err != nil {
		_ = m.DisposeDevices()
		return nil, fmt.Errorf("dualstack: set v6 addr %s on %s: %w", cidr6, m.ifName, err)
	}

	// Install split routes for both families.
	_ = m.rtc4.DelSplit(m.ifName)
	if err := m.rtc4.AddSplit(m.ifName); err != nil {
		_ = m.DisposeDevices()
		return nil, fmt.Errorf("dualstack: add v4 split: %w", err)
	}
	_ = m.rtc6.DelSplit(m.ifName)
	if err := m.rtc6.AddSplit(m.ifName); err != nil {
		_ = m.DisposeDevices()
		return nil, fmt.Errorf("dualstack: add v6 split: %w", err)
	}

	m.tunDev = utun.NewDarwinTunDevice(raw)
	return m.tunDev, nil
}

func (m *dualStack) DisposeDevices() error {
	_ = m.rtc4.DelSplit(m.ifName)
	_ = m.rtc6.DelSplit(m.ifName)
	if m.resolvedRouteIP4 != "" {
		_ = m.rtc4.Del(m.resolvedRouteIP4)
	}
	if m.resolvedRouteIP6 != "" {
		_ = m.rtc6.Del(m.resolvedRouteIP6)
	}
	if m.tunDev != nil {
		_ = m.tunDev.Close() // closes underlying rawUTUN
	} else if m.rawUTUN != nil {
		_ = m.rawUTUN.Close() // tunDev never created, close raw directly
	}
	m.tunDev = nil
	m.rawUTUN = nil
	m.ifName = ""
	return nil
}

func (m *dualStack) validateSettings() error {
	if m.s.Server.IsZero() {
		return fmt.Errorf("dualstack: empty Server")
	}
	if !m.s.IPv4.IsValid() || !m.s.IPv4.Unmap().Is4() {
		return fmt.Errorf("dualstack: invalid IPv4 %q", m.s.IPv4)
	}
	if !m.s.IPv6.IsValid() || m.s.IPv6.Unmap().Is4() {
		return fmt.Errorf("dualstack: invalid IPv6 %q", m.s.IPv6)
	}
	return nil
}

func (m *dualStack) effectiveMTU() int {
	mtu := m.s.MTU
	if mtu <= 0 {
		mtu = settings.SafeMTU
	}
	// Must satisfy both IPv4 (68) and IPv6 (1280) minimums.
	if mtu < settings.MinimumIPv6MTU {
		mtu = settings.MinimumIPv6MTU
	}
	return mtu
}

func (m *dualStack) resolveRouteIPv4() (string, error) {
	if m.routeEndpoint.IsValid() {
		ip := m.routeEndpoint.Addr()
		if ip.Unmap().Is4() {
			return ip.Unmap().String(), nil
		}
		return "", fmt.Errorf("route endpoint %s is IPv6, expected IPv4", ip)
	}
	return m.s.Server.RouteIPv4()
}

func (m *dualStack) resolveRouteIPv6() (string, error) {
	if m.routeEndpoint.IsValid() {
		ip := m.routeEndpoint.Addr()
		if !ip.Unmap().Is4() {
			return ip.String(), nil
		}
		return "", fmt.Errorf("route endpoint %s is IPv4, expected IPv6", ip)
	}
	return m.s.Server.RouteIPv6()
}

func shouldSkipDarwinIPv4Route(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "expected IPv4") ||
		strings.Contains(msg, "no matching address family found")
}

func shouldSkipDarwinIPv6Route(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "expected IPv6") ||
		strings.Contains(msg, "no matching address family found")
}
