//go:build darwin

package manager

import (
	"fmt"
	"net"
	"strings"

	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL/darwin/network_tools/ifconfig"
	"tungo/infrastructure/PAL/darwin/network_tools/route"
	"tungo/infrastructure/PAL/darwin/utun"
	"tungo/infrastructure/settings"
)

type v6 struct {
	s          settings.Settings
	tunDev     tun.Device
	rawUTUN    utun.UTUN
	ifc        ifconfig.Contract // v6 impl
	rt         route.Contract    // v6 impl
	ifName     string
	addedSplit bool
}

func newV6(s settings.Settings, ifc ifconfig.Contract, rt route.Contract) *v6 {
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

	if err := m.rt.Get(m.s.ConnectionIP); err != nil {
		_ = m.DisposeDevices()
		return nil, fmt.Errorf("route to server %s: %w", m.s.ConnectionIP, err)
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
	m.addedSplit = true

	m.tunDev = utun.NewDarwinTunDevice(raw)
	return m.tunDev, nil
}

func (m *v6) DisposeDevices() error {
	_ = m.rt.DelSplit(m.ifName)
	if m.s.ConnectionIP != "" {
		_ = m.rt.Del(m.s.ConnectionIP)
	}
	if m.tunDev != nil {
		_ = m.tunDev.Close()
		m.tunDev = nil
	}
	m.rawUTUN = nil
	m.ifName = ""
	m.addedSplit = false
	return nil
}

func (m *v6) validateSettings() error {
	ip := net.ParseIP(m.s.InterfaceAddress)
	if ip == nil || ip.To4() != nil {
		return fmt.Errorf("v6: invalid InterfaceAddress %q", m.s.InterfaceAddress)
	}
	dst := net.ParseIP(m.s.ConnectionIP)
	if dst == nil || dst.To4() != nil {
		return fmt.Errorf("v6: invalid ConnectionIP %q", m.s.ConnectionIP)
	}
	if m.s.InterfaceIPCIDR != "" {
		if !strings.Contains(m.s.InterfaceIPCIDR, "/") {
			return fmt.Errorf("v6: InterfaceIPCIDR must be CIDR or empty, got %q", m.s.InterfaceIPCIDR)
		}
		if _, _, err := net.ParseCIDR(m.s.InterfaceIPCIDR); err != nil {
			return fmt.Errorf("v6: bad InterfaceIPCIDR %q: %w", m.s.InterfaceIPCIDR, err)
		}
	}
	return nil
}

func (m *v6) assignIPv6() error {
	cidr := m.s.InterfaceIPCIDR
	if cidr == "" {
		cidr = m.s.InterfaceAddress + "/128"
	} else {
		parts := strings.Split(cidr, "/")
		if len(parts) != 2 {
			return fmt.Errorf("v6: malformed CIDR %q", cidr)
		}
		cidr = m.s.InterfaceAddress + "/" + parts[1]
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
