//go:build darwin

package manager

import (
	"fmt"
	"io"
	"net/netip"

	"tungo/internal/config/settings"
	"tungo/internal/tun/internal/darwin/utun"
)

type v6 struct {
	s               settings.Settings
	tunDev          io.ReadWriteCloser
	rawUTUN         utun.UTUN
	ifc             interfaceConfigurator
	rt              routeConfigurator
	ifName          string
	resolvedRouteIP string // cached resolved server IP for consistent teardown
}

func newV6(
	s settings.Settings,
	ifc interfaceConfigurator,
	rt routeConfigurator,
) *v6 {
	return &v6{
		s:   s,
		ifc: ifc,
		rt:  rt,
	}
}

func (m *v6) OpenTunnel(serverAddr netip.Addr) (io.ReadWriteCloser, error) {
	if !serverAddr.IsValid() {
		return nil, fmt.Errorf("v6: invalid server address %q", serverAddr)
	}
	serverAddr = serverAddr.Unmap()

	raw, err := utun.Create(m.ifc, m.s.MTU)
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

	if !serverAddr.Is4() {
		routeIP := serverAddr.String()
		if err := m.rt.Get(routeIP); err != nil {
			_ = m.CloseTunnel()
			return nil, fmt.Errorf("route to server %s: %w", routeIP, err)
		}
		m.resolvedRouteIP = routeIP
	}
	if err := m.assignIPv6(); err != nil {
		_ = m.CloseTunnel()
		return nil, err
	}
	_ = m.rt.DelSplit(m.ifName)
	if err := m.rt.AddSplit(m.ifName); err != nil {
		_ = m.CloseTunnel()
		return nil, fmt.Errorf("add v6 split default: %w", err)
	}

	m.tunDev = utun.NewDarwinTunDevice(raw)
	return m.tunDev, nil
}

func (m *v6) CloseTunnel() error {
	if m.ifName != "" {
		_ = m.rt.DelSplit(m.ifName)
	}
	if m.resolvedRouteIP != "" {
		_ = m.rt.Del(m.resolvedRouteIP)
	}
	if m.tunDev != nil {
		_ = m.tunDev.Close() // closes underlying rawUTUN
	} else if m.rawUTUN != nil {
		_ = m.rawUTUN.Close() // tunDev never created, close raw directly
	}
	m.tunDev = nil
	m.rawUTUN = nil
	m.ifName = ""
	m.resolvedRouteIP = ""
	return nil
}

func (m *v6) assignIPv6() error {
	prefix := netip.PrefixFrom(m.s.IPv6, m.s.IPv6Subnet.Bits())
	if err := m.ifc.LinkAddrAdd(m.ifName, prefix); err != nil {
		return fmt.Errorf("v6: set addr %s on %s: %w", prefix, m.ifName, err)
	}
	return nil
}
