//go:build darwin

package manager

import (
	"fmt"
	"io"
	"net/netip"

	"tungo/internal/config/settings"
	"tungo/internal/tun/internal/darwin/utun"
)

type v4 struct {
	s               settings.Settings
	tunDev          io.ReadWriteCloser
	rawUTUN         utun.UTUN
	ifc             interfaceConfigurator
	rtc             routeConfigurator
	ifName          string
	resolvedRouteIP string // cached resolved server IP for consistent teardown
}

func newV4(
	s settings.Settings,
	ifc interfaceConfigurator,
	rt routeConfigurator,
) *v4 {
	return &v4{
		s:   s,
		ifc: ifc,
		rtc: rt,
	}
}

func (m *v4) OpenTunnel(serverAddr netip.Addr) (io.ReadWriteCloser, error) {
	if !serverAddr.IsValid() {
		return nil, fmt.Errorf("v4: invalid server address %q", serverAddr)
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
	// The server address belongs to the outer transport and may use a
	// different IP family than the TUN. Pin it only when this manager's
	// split routes cover that family.
	if serverAddr.Is4() {
		routeIP := serverAddr.String()
		if addErr := m.rtc.Add(routeIP); addErr != nil {
			_ = m.CloseTunnel()
			return nil, fmt.Errorf("route to server %s: %w", routeIP, addErr)
		}
		m.resolvedRouteIP = routeIP
	}
	if assignErr := m.assignIPv4(); assignErr != nil {
		_ = m.CloseTunnel()
		return nil, assignErr
	}
	_ = m.rtc.DelSplit(m.ifName)
	if addErr := m.rtc.AddSplit(m.ifName); addErr != nil {
		_ = m.CloseTunnel()
		return nil, fmt.Errorf("add v4 split default: %w", addErr)
	}

	m.tunDev = utun.NewDarwinTunDevice(raw)
	return m.tunDev, nil
}

func (m *v4) CloseTunnel() error {
	if m.ifName != "" {
		_ = m.rtc.DelSplit(m.ifName)
	}
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
	m.resolvedRouteIP = ""
	return nil
}

func (m *v4) assignIPv4() error {
	prefix := netip.PrefixFrom(m.s.IPv4, 32)
	if err := m.ifc.LinkAddrAdd(m.ifName, prefix); err != nil {
		return fmt.Errorf("v4: set addr %s on %s: %w", prefix, m.ifName, err)
	}
	return nil
}
