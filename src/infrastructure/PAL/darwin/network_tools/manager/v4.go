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

type v4 struct {
	s              settings.Settings
	tunDev         tun.Device
	rawUTUN        utun.UTUN
	ifc            ifconfig.Contract // v4 ifconfig.Contract implementation
	rtc            route.Contract    // v4 route.Contract implementation
	ifName         string
	resolvedRouteIP string // cached resolved server IP for consistent teardown
}

func newV4(
	s settings.Settings,
	ifc ifconfig.Contract,
	rt route.Contract,
) *v4 {
	return &v4{
		s:   s,
		ifc: ifc,
		rtc: rt,
	}
}

func (m *v4) CreateDevice() (tun.Device, error) {
	if err := m.validateSettings(); err != nil {
		return nil, err
	}
	raw, err := utun.NewDefaultFactory(m.ifc).CreateTUN(m.effectiveMTU())
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
	routeIP, routeErr := m.s.Host.RouteIPv4()
	if routeErr != nil {
		_ = m.DisposeDevices()
		return nil, fmt.Errorf("v4: resolve route for %s: %w", m.s.Host, routeErr)
	}
	m.resolvedRouteIP = routeIP
	if getErr := m.rtc.Get(routeIP); getErr != nil {
		_ = m.DisposeDevices()
		return nil, fmt.Errorf("route to server %s: %w", m.s.Host, getErr)
	}
	if assignErr := m.assignIPv4(); assignErr != nil {
		_ = m.DisposeDevices()
		return nil, assignErr
	}
	_ = m.rtc.DelSplit(m.ifName)
	if addErr := m.rtc.AddSplit(m.ifName); addErr != nil {
		_ = m.DisposeDevices()
		return nil, fmt.Errorf("add v4 split default: %w", addErr)
	}

	m.tunDev = utun.NewDarwinTunDevice(raw)
	return m.tunDev, nil
}

func (m *v4) DisposeDevices() error {
	_ = m.rtc.DelSplit(m.ifName)
	if m.resolvedRouteIP != "" {
		_ = m.rtc.Del(m.resolvedRouteIP)
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

func (m *v4) validateSettings() error {
	if m.s.Host.IsZero() {
		return fmt.Errorf("v4: empty Host")
	}
	if !m.s.IPv4IP.IsValid() || !m.s.IPv4IP.Unmap().Is4() {
		return fmt.Errorf("v4: invalid IPv4IP %q", m.s.IPv4IP)
	}
	if !m.s.IPv4Subnet.IsValid() {
		return fmt.Errorf("v4: invalid IPv4Subnet %q", m.s.IPv4Subnet)
	}
	return nil
}

func (m *v4) assignIPv4() error {
	cidr := fmt.Sprintf("%s/32", m.s.IPv4IP)
	if err := m.ifc.LinkAddrAdd(m.ifName, cidr); err != nil {
		return fmt.Errorf("v4: set addr %s on %s: %w", cidr, m.ifName, err)
	}
	return nil
}

func (m *v4) effectiveMTU() int {
	mtu := m.s.MTU
	if mtu <= 0 {
		mtu = settings.SafeMTU
	}
	if mtu < settings.MinimumIPv4MTU {
		mtu = settings.MinimumIPv4MTU
	}
	return mtu
}
