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
	tun              tunnel
	ifc4             interfaceConfigurator
	ifc6             interfaceConfigurator
	rtc4             routeConfigurator
	rtc6             routeConfigurator
	pinnedServerAddr netip.Addr
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

func (m *dualStack) OpenTunnel(serverAddr netip.Addr) (io.ReadWriter, error) {
	if !serverAddr.IsValid() {
		return nil, fmt.Errorf("dualstack: invalid server address %q", serverAddr)
	}
	serverAddr = serverAddr.Unmap()
	tun, err := utun.New()
	if err != nil {
		return nil, fmt.Errorf("create utun: %w", err)
	}
	m.tun = tun
	if err := m.ifc4.SetMTU(m.tun.Name(), m.s.MTU); err != nil {
		openErr := fmt.Errorf("set MTU %d on %s: %w", m.s.MTU, m.tun.Name(), err)
		_ = m.CloseTunnel()
		return nil, openErr
	}
	if serverAddr.Is4() {
		routeIP := serverAddr.String()
		if err := m.rtc4.Add(routeIP); err != nil {
			_ = m.CloseTunnel()
			return nil, fmt.Errorf("dualstack: pin v4 route to %s: %w", routeIP, err)
		}
		m.pinnedServerAddr = serverAddr
	} else {
		routeIP := serverAddr.String()
		if err := m.rtc6.Add(routeIP); err != nil {
			_ = m.CloseTunnel()
			return nil, fmt.Errorf("dualstack: pin v6 route to %s: %w", routeIP, err)
		}
		m.pinnedServerAddr = serverAddr
	}
	if err := assignIPv4(m.ifc4, m.tun, m.s.IPv4); err != nil {
		_ = m.CloseTunnel()
		return nil, fmt.Errorf("dualstack: %w", err)
	}
	if err := assignIPv6(m.ifc6, m.tun, m.s.IPv6, m.s.IPv6Subnet); err != nil {
		_ = m.CloseTunnel()
		return nil, fmt.Errorf("dualstack: %w", err)
	}
	// Install split routes for both families.
	_ = m.rtc4.DelSplit(m.tun.Name())
	if err := m.rtc4.AddSplit(m.tun.Name()); err != nil {
		_ = m.CloseTunnel()
		return nil, fmt.Errorf("dualstack: add v4 split: %w", err)
	}
	_ = m.rtc6.DelSplit(m.tun.Name())
	if err := m.rtc6.AddSplit(m.tun.Name()); err != nil {
		_ = m.CloseTunnel()
		return nil, fmt.Errorf("dualstack: add v6 split: %w", err)
	}
	return m.tun, nil
}

func (m *dualStack) CloseTunnel() error {
	if m.tun != nil {
		_ = m.rtc4.DelSplit(m.tun.Name())
		_ = m.rtc6.DelSplit(m.tun.Name())
	}
	if m.pinnedServerAddr.IsValid() {
		if m.pinnedServerAddr.Is4() {
			_ = m.rtc4.Del(m.pinnedServerAddr.String())
		} else {
			_ = m.rtc6.Del(m.pinnedServerAddr.String())
		}
	}
	if m.tun != nil {
		_ = m.tun.Close()
	}
	m.tun = nil
	m.pinnedServerAddr = netip.Addr{}
	return nil
}
