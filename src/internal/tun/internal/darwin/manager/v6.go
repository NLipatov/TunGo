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
	s                settings.Settings
	tun              tunnel
	ifc              interfaceConfigurator
	rt               routeConfigurator
	pinnedServerAddr netip.Addr
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

func (m *v6) OpenTunnel(serverAddr netip.Addr) (io.ReadWriter, error) {
	if !serverAddr.IsValid() {
		return nil, fmt.Errorf("v6: invalid server address %q", serverAddr)
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
	if !serverAddr.Is4() {
		routeIP := serverAddr.String()
		if err := m.rt.Add(routeIP); err != nil {
			_ = m.CloseTunnel()
			return nil, fmt.Errorf("route to server %s: %w", routeIP, err)
		}
		m.pinnedServerAddr = serverAddr
	}
	if err := assignIPv6(m.ifc, m.tun, m.s.IPv6, m.s.IPv6Subnet); err != nil {
		_ = m.CloseTunnel()
		return nil, err
	}
	_ = m.rt.DelSplit(m.tun.Name())
	if err := m.rt.AddSplit(m.tun.Name()); err != nil {
		_ = m.CloseTunnel()
		return nil, fmt.Errorf("add v6 split default: %w", err)
	}
	return m.tun, nil
}

func assignIPv6(ifc interfaceConfigurator, tun tunnel, addr netip.Addr, subnet netip.Prefix) error {
	prefix := netip.PrefixFrom(addr, subnet.Bits())
	if err := ifc.LinkAddrAdd(tun.Name(), prefix); err != nil {
		return fmt.Errorf("v6: set addr %s on %s: %w", prefix, tun.Name(), err)
	}
	return nil
}

func (m *v6) CloseTunnel() error {
	if m.tun != nil {
		_ = m.rt.DelSplit(m.tun.Name())
	}
	if m.pinnedServerAddr.IsValid() {
		_ = m.rt.Del(m.pinnedServerAddr.String())
	}
	if m.tun != nil {
		_ = m.tun.Close()
	}
	m.tun = nil
	m.pinnedServerAddr = netip.Addr{}
	return nil
}
