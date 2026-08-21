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
	s                settings.Settings
	tun              tunnel
	ifc              interfaceConfigurator
	rtc              routeConfigurator
	pinnedServerAddr netip.Addr
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

func (m *v4) OpenTunnel(serverAddr netip.Addr) (io.ReadWriter, error) {
	if !serverAddr.IsValid() {
		return nil, fmt.Errorf("v4: invalid server address %q", serverAddr)
	}
	serverAddr = serverAddr.Unmap()
	tun, err := utun.New()
	if err != nil {
		return nil, fmt.Errorf("create utun: %w", err)
	}
	m.tun = tun
	if err := m.ifc.SetMTU(m.tun.Name(), m.s.MTU); err != nil {
		openErr := fmt.Errorf("set MTU %d on %s: %w", m.s.MTU, m.tun.Name(), err)
		_ = m.CloseTunnel()
		return nil, openErr
	}
	// The server address belongs to the outer transport and may use a
	// different IP family than the TUN. Pin it only when this manager's
	// split routes cover that family.
	if serverAddr.Is4() {
		routeIP := serverAddr.String()
		if addErr := m.rtc.Add(routeIP); addErr != nil {
			_ = m.CloseTunnel()
			return nil, fmt.Errorf("route to server %s: %w", routeIP, addErr)
		}
		m.pinnedServerAddr = serverAddr
	}
	if err := assignIPv4(m.ifc, m.tun, m.s.IPv4); err != nil {
		_ = m.CloseTunnel()
		return nil, err
	}
	_ = m.rtc.DelSplit(m.tun.Name())
	if addErr := m.rtc.AddSplit(m.tun.Name()); addErr != nil {
		_ = m.CloseTunnel()
		return nil, fmt.Errorf("add v4 split default: %w", addErr)
	}
	return m.tun, nil
}

func assignIPv4(ifc interfaceConfigurator, tun tunnel, addr netip.Addr) error {
	prefix := netip.PrefixFrom(addr, 32)
	if err := ifc.LinkAddrAdd(tun.Name(), prefix); err != nil {
		return fmt.Errorf("v4: set addr %s on %s: %w", prefix, tun.Name(), err)
	}
	return nil
}

func (m *v4) CloseTunnel() error {
	if m.tun != nil {
		_ = m.rtc.DelSplit(m.tun.Name())
	}
	if m.pinnedServerAddr.IsValid() {
		_ = m.rtc.Del(m.pinnedServerAddr.String())
	}
	if m.tun != nil {
		_ = m.tun.Close()
	}
	m.tun = nil
	m.pinnedServerAddr = netip.Addr{}
	return nil
}
