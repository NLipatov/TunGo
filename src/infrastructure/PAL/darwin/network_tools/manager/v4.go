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

type v4 struct {
	s          settings.Settings
	tunDev     tun.Device
	rawUTUN    utun.UTUN
	ifc        ifconfig.Contract // v4 impl
	rtc        route.Contract    // v4 impl
	ifName     string
	addedSplit bool
}

func newV4(
	s settings.Settings,
	ifc ifconfig.Contract, // from ifconfig.NewFactory(...).NewV4()
	rt route.Contract, // from route.NewFactory(...).NewV4()
) *v4 {
	return &v4{s: s, ifc: ifc, rtc: rt}
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
	if err := m.rtc.Get(m.s.ConnectionIP); err != nil {
		_ = m.DisposeDevices()
		return nil, fmt.Errorf("route to server %s: %w", m.s.ConnectionIP, err)
	}
	if err := m.assignIPv4(); err != nil {
		_ = m.DisposeDevices()
		return nil, err
	}
	if err := m.rtc.AddSplit(m.ifName); err != nil {
		_ = m.DisposeDevices()
		return nil, fmt.Errorf("add v4 split default: %w", err)
	}
	m.addedSplit = true
	m.tunDev = utun.NewDarwinTunDevice(raw)
	return m.tunDev, nil
}

func (m *v4) DisposeDevices() error {
	if m.addedSplit && m.ifName != "" {
		_ = m.rtc.DelSplit(m.ifName)
	}
	if m.s.ConnectionIP != "" {
		_ = m.rtc.Del(m.s.ConnectionIP)
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

func (m *v4) validateSettings() error {
	if net.ParseIP(m.s.ConnectionIP) == nil || net.ParseIP(m.s.ConnectionIP).To4() == nil {
		return fmt.Errorf("v4: invalid ConnectionIP %q", m.s.ConnectionIP)
	}
	if ip := net.ParseIP(m.s.InterfaceAddress); ip == nil || ip.To4() == nil {
		return fmt.Errorf("v4: invalid InterfaceAddress %q", m.s.InterfaceAddress)
	}
	if !strings.Contains(m.s.InterfaceIPCIDR, "/") {
		return fmt.Errorf("v4: InterfaceIPCIDR must be CIDR, got %q", m.s.InterfaceIPCIDR)
	}
	if _, _, err := net.ParseCIDR(m.s.InterfaceIPCIDR); err != nil {
		return fmt.Errorf("v4: bad InterfaceIPCIDR %q: %w", m.s.InterfaceIPCIDR, err)
	}
	return nil
}

func (m *v4) assignIPv4() error {
	ip := net.ParseIP(m.s.InterfaceAddress)
	_, nw, _ := net.ParseCIDR(m.s.InterfaceIPCIDR)
	if ip == nil || nw == nil || !nw.Contains(ip) {
		return fmt.Errorf("v4: address %s not in %s", m.s.InterfaceAddress, m.s.InterfaceIPCIDR)
	}
	parts := strings.Split(m.s.InterfaceIPCIDR, "/")
	cidr := fmt.Sprintf("%s/%s", m.s.InterfaceAddress, parts[1])
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
