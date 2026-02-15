//go:build darwin

package manager

import (
	"fmt"

	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL/darwin/network_tools/ifconfig"
	"tungo/infrastructure/PAL/darwin/network_tools/route"
	"tungo/infrastructure/PAL/darwin/utun"
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
	resolvedRouteIP4 string // cached resolved IPv4 server IP for consistent teardown
	resolvedRouteIP6 string // cached resolved IPv6 server IP for consistent teardown
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
	routeIP4, err := m.s.Host.RouteIPv4()
	if err != nil {
		_ = m.DisposeDevices()
		return nil, fmt.Errorf("dualstack: resolve v4 route for %s: %w", m.s.Host, err)
	}
	m.resolvedRouteIP4 = routeIP4
	if err := m.rtc4.Get(routeIP4); err != nil {
		_ = m.DisposeDevices()
		return nil, fmt.Errorf("dualstack: pin v4 route to %s: %w", m.s.Host, err)
	}

	// Pin route to IPv6 server (if configured).
	if !m.s.IPv6Host.IsZero() {
		routeIP6, err := m.s.IPv6Host.RouteIPv6()
		if err != nil {
			_ = m.DisposeDevices()
			return nil, fmt.Errorf("dualstack: resolve v6 route for %s: %w", m.s.IPv6Host, err)
		}
		m.resolvedRouteIP6 = routeIP6
		if err := m.rtc6.Get(routeIP6); err != nil {
			_ = m.DisposeDevices()
			return nil, fmt.Errorf("dualstack: pin v6 route to %s: %w", m.s.IPv6Host, err)
		}
	}

	// Assign IPv4 address.
	cidr4 := fmt.Sprintf("%s/32", m.s.InterfaceIP)
	if err := m.ifc4.LinkAddrAdd(m.ifName, cidr4); err != nil {
		_ = m.DisposeDevices()
		return nil, fmt.Errorf("dualstack: set v4 addr %s on %s: %w", cidr4, m.ifName, err)
	}

	// Assign IPv6 address.
	cidr6 := m.ipv6CIDR()
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
	if m.s.Host.IsZero() {
		return fmt.Errorf("dualstack: empty Host")
	}
	if !m.s.InterfaceIP.IsValid() || !m.s.InterfaceIP.Unmap().Is4() {
		return fmt.Errorf("dualstack: invalid IPv4 InterfaceIP %q", m.s.InterfaceIP)
	}
	if !m.s.IPv6IP.IsValid() || m.s.IPv6IP.Unmap().Is4() {
		return fmt.Errorf("dualstack: invalid IPv6IP %q", m.s.IPv6IP)
	}
	return nil
}

func (m *dualStack) ipv6CIDR() string {
	if m.s.IPv6Subnet.IsValid() {
		return fmt.Sprintf("%s/%d", m.s.IPv6IP, m.s.IPv6Subnet.Bits())
	}
	return fmt.Sprintf("%s/128", m.s.IPv6IP)
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
