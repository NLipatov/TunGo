//go:build darwin

package manager

import (
	"fmt"
	"io"
	"net/netip"

	"tungo/internal/config/settings"
	"tungo/internal/tun/internal/darwin/utun"
)

// dualStack manages a single utun device with both IPv4 and IPv6 addresses and routes.
// macOS utun natively supports dual-stack via its AF header — no need for two devices.
type dualStack struct {
	s                settings.Settings
	tunDev           io.ReadWriteCloser
	rawUTUN          utun.UTUN
	ifc4             interfaceConfigurator
	ifc6             interfaceConfigurator
	rtc4             routeConfigurator
	rtc6             routeConfigurator
	ifName           string
	resolvedRouteIP4 string // cached resolved IPv4 server IP for consistent teardown
	resolvedRouteIP6 string // cached resolved IPv6 server IP for consistent teardown
}

func newDualStack(
	s settings.Settings,
	ifc4 interfaceConfigurator,
	ifc6 interfaceConfigurator,
	rtc4 routeConfigurator,
	rtc6 routeConfigurator,
) *dualStack {
	return &dualStack{
		s:    s,
		ifc4: ifc4,
		ifc6: ifc6,
		rtc4: rtc4,
		rtc6: rtc6,
	}
}

func (m *dualStack) OpenTunnel(serverAddr netip.Addr) (io.ReadWriteCloser, error) {
	if !serverAddr.IsValid() {
		return nil, fmt.Errorf("dualstack: invalid server address %q", serverAddr)
	}
	serverAddr = serverAddr.Unmap()

	raw, err := utun.Create(m.ifc4, m.s.MTU)
	if err != nil {
		return nil, fmt.Errorf("create utun: %w", err)
	}
	m.rawUTUN = raw

	name, err := raw.Name()
	if err != nil {
		_ = m.CloseTunnel()
		return nil, fmt.Errorf("get utun name: %w", err)
	}
	m.ifName = name

	if serverAddr.Is4() {
		routeIP := serverAddr.String()
		if err := m.rtc4.Add(routeIP); err != nil {
			_ = m.CloseTunnel()
			return nil, fmt.Errorf("dualstack: pin v4 route to %s: %w", routeIP, err)
		}
		m.resolvedRouteIP4 = routeIP
	} else {
		routeIP := serverAddr.String()
		if err := m.rtc6.Add(routeIP); err != nil {
			_ = m.CloseTunnel()
			return nil, fmt.Errorf("dualstack: pin v6 route to %s: %w", routeIP, err)
		}
		m.resolvedRouteIP6 = routeIP
	}

	// Assign IPv4 address.
	prefix4 := netip.PrefixFrom(m.s.IPv4, 32)
	if err := m.ifc4.LinkAddrAdd(m.ifName, prefix4); err != nil {
		_ = m.CloseTunnel()
		return nil, fmt.Errorf("dualstack: set v4 addr %s on %s: %w", prefix4, m.ifName, err)
	}

	// Assign IPv6 address.
	prefix6 := netip.PrefixFrom(m.s.IPv6, m.s.IPv6Subnet.Bits())
	if err := m.ifc6.LinkAddrAdd(m.ifName, prefix6); err != nil {
		_ = m.CloseTunnel()
		return nil, fmt.Errorf("dualstack: set v6 addr %s on %s: %w", prefix6, m.ifName, err)
	}

	// Install split routes for both families.
	_ = m.rtc4.DelSplit(m.ifName)
	if err := m.rtc4.AddSplit(m.ifName); err != nil {
		_ = m.CloseTunnel()
		return nil, fmt.Errorf("dualstack: add v4 split: %w", err)
	}
	_ = m.rtc6.DelSplit(m.ifName)
	if err := m.rtc6.AddSplit(m.ifName); err != nil {
		_ = m.CloseTunnel()
		return nil, fmt.Errorf("dualstack: add v6 split: %w", err)
	}

	m.tunDev = utun.NewDarwinTunDevice(raw)
	return m.tunDev, nil
}

func (m *dualStack) CloseTunnel() error {
	if m.ifName != "" {
		_ = m.rtc4.DelSplit(m.ifName)
		_ = m.rtc6.DelSplit(m.ifName)
	}
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
	m.resolvedRouteIP4 = ""
	m.resolvedRouteIP6 = ""
	return nil
}
