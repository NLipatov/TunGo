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

type v6 struct {
	s       settings.Settings
	tunDev  tun.Device
	rawUTUN utun.UTUN
	ifc     ifconfig.Contract // v6 ifconfig.Contract implementation
	rt      route.Contract    // v6 route.Contract implementation
	ifName  string
}

func newV6(
	s settings.Settings,
	ifc ifconfig.Contract,
	rt route.Contract,
) *v6 {
	return &v6{
		s:   s,
		ifc: ifc,
		rt:  rt,
	}
}

func (m *v6) CreateDevice() (tun.Device, error) {
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

	routeIP, routeErr := m.s.Host.RouteIP()
	if routeErr != nil {
		_ = m.DisposeDevices()
		return nil, fmt.Errorf("v6: resolve route for %s: %w", m.s.Host, routeErr)
	}
	if err := m.rt.Get(routeIP); err != nil {
		_ = m.DisposeDevices()
		return nil, fmt.Errorf("route to server %s: %w", m.s.Host, err)
	}
	if err := m.assignIPv6(); err != nil {
		_ = m.DisposeDevices()
		return nil, err
	}
	_ = m.rt.DelSplit(m.ifName)
	if err := m.rt.AddSplit(m.ifName); err != nil {
		_ = m.DisposeDevices()
		return nil, fmt.Errorf("add v6 split default: %w", err)
	}

	m.tunDev = utun.NewDarwinTunDevice(raw)
	return m.tunDev, nil
}

func (m *v6) DisposeDevices() error {
	_ = m.rt.DelSplit(m.ifName)
	if !m.s.Host.IsZero() {
		_ = m.rt.Del(string(m.s.Host))
	}
	if m.tunDev != nil {
		_ = m.tunDev.Close()
		m.tunDev = nil
	}
	m.rawUTUN = nil
	m.ifName = ""
	return nil
}

func (m *v6) validateSettings() error {
	ip := m.s.InterfaceIP.Unmap()
	if !ip.IsValid() || ip.Is4() {
		return fmt.Errorf("v6: invalid InterfaceIP %q", m.s.InterfaceIP)
	}
	if m.s.Host.IsZero() {
		return fmt.Errorf("v6: empty Host")
	}
	return nil
}

func (m *v6) assignIPv6() error {
	var cidr string
	if m.s.InterfaceSubnet.IsValid() {
		cidr = fmt.Sprintf("%s/%d", m.s.InterfaceIP, m.s.InterfaceSubnet.Bits())
	} else {
		cidr = fmt.Sprintf("%s/128", m.s.InterfaceIP)
	}
	if err := m.ifc.LinkAddrAdd(m.ifName, cidr); err != nil {
		return fmt.Errorf("v6: set addr %s on %s: %w", cidr, m.ifName, err)
	}
	return nil
}

func (m *v6) effectiveMTU() int {
	mtu := m.s.MTU
	if mtu <= 0 {
		mtu = settings.SafeMTU
	}
	if mtu < 1280 {
		mtu = 1280
	}
	return mtu
}
