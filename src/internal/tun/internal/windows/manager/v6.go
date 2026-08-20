//go:build windows

package manager

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"tungo/internal/config/settings"
	"tungo/internal/tun/internal/windows/wtun"

	"golang.zx2c4.com/wintun"
)

// v6Manager configures a Wintun adapter and the host stack for IPv6.
type v6Manager struct {
	s               settings.Settings
	tun             io.ReadWriteCloser
	netConfig       networkConfigurator
	resolvedRouteIP netip.Addr
	resolvedRouteIf string // cached egress interface used for host route
}

func newV6Manager(
	s settings.Settings,
	netConfig networkConfigurator,
) *v6Manager {
	return &v6Manager{
		s:         s,
		netConfig: netConfig,
	}
}

// OpenTunnel creates/configures the TUN adapter and system routes/DNS for IPv6.
// Safe order mirrors v4 with IPv6-specific details.
func (m *v6Manager) OpenTunnel(serverAddr netip.Addr) (io.ReadWriteCloser, error) {
	if !serverAddr.IsValid() {
		return nil, fmt.Errorf("invalid server address %q", serverAddr)
	}
	serverAddr = serverAddr.Unmap()

	tunDev, err := m.createTunDevice()
	if err != nil {
		return nil, err
	}
	m.tun = tunDev

	if err = m.addStaticRouteToServer(serverAddr); err != nil {
		_ = m.CloseTunnel()
		return nil, err
	}
	if err = m.assignIPToTunDevice(); err != nil {
		_ = m.CloseTunnel()
		return nil, err
	}
	if err = m.setRouteToTunDevice(); err != nil {
		_ = m.CloseTunnel()
		return nil, err
	}
	if err = m.netConfig.SetMTU(m.s.TunName, m.s.MTU); err != nil {
		_ = m.CloseTunnel()
		return nil, err
	}
	if err = m.setDNSToTunDevice(); err != nil {
		_ = m.CloseTunnel()
		return nil, err
	}
	return m.tun, nil
}

func (m *v6Manager) createTunDevice() (io.ReadWriteCloser, error) {
	adapter, err := wintun.CreateAdapter(m.s.TunName, tunnelType, nil)
	if err != nil {
		if existing, openErr := wintun.OpenAdapter(m.s.TunName); openErr == nil {
			return wtun.NewTUN(existing)
		}
		return nil, fmt.Errorf("create/open adapter: %w", err)
	}
	dev, devErr := wtun.NewTUN(adapter)
	if devErr != nil {
		_ = adapter.Close()
		return nil, devErr
	}
	return dev, nil
}

func (m *v6Manager) addStaticRouteToServer(serverAddr netip.Addr) error {
	if serverAddr.Is4() {
		return nil
	}
	var err error
	gw, ifName, ifIndex, _, bestErr := m.netConfig.BestRoute(serverAddr)
	if bestErr != nil {
		return bestErr
	}
	ifName, err = routeInterfaceName(ifName, ifIndex)
	if err != nil {
		return err
	}
	_ = m.netConfig.DeleteRoute(serverAddr)
	_ = m.netConfig.DeleteRouteOnInterface(serverAddr, ifName)
	var addErr error
	if !gw.IsValid() {
		// on-link
		addErr = m.netConfig.AddHostRouteOnLink(serverAddr, ifName)
	} else {
		addErr = m.netConfig.AddHostRouteViaGateway(serverAddr, ifName, gw)
	}
	if addErr != nil {
		return addErr
	}
	m.resolvedRouteIP = serverAddr
	m.resolvedRouteIf = ifName
	return nil
}

func (m *v6Manager) assignIPToTunDevice() error {
	prefix := netip.PrefixFrom(m.s.IPv6, m.s.IPv6Subnet.Bits())
	return m.netConfig.SetAddressStatic(m.s.TunName, prefix)
}

// setRouteToTunDevice replaces any existing default with IPv6 split default (::/1, 8000::/1).
func (m *v6Manager) setRouteToTunDevice() error {
	_ = m.netConfig.DeleteDefaultSplitRoutes(m.s.TunName)
	return m.netConfig.AddDefaultSplitRoutes(m.s.TunName)
}

func (m *v6Manager) setDNSToTunDevice() error {
	if err := m.netConfig.SetDNS(m.s.TunName, m.s.DNSv6Resolvers()); err != nil {
		return err
	}
	if err := m.netConfig.FlushDNS(); err != nil {
		slog.Warn("failed to flush IPv6 DNS cache", "err", err)
	}
	return nil
}

// CloseTunnel reverses OpenTunnel in safe order.
func (m *v6Manager) CloseTunnel() error {
	var cleanupErrs []error
	if err := m.netConfig.DeleteDefaultSplitRoutes(m.s.TunName); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("delete default split routes: %w", err))
	}
	if m.resolvedRouteIP.IsValid() {
		if err := m.netConfig.DeleteRouteOnInterface(m.resolvedRouteIP, m.resolvedRouteIf); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("delete route %s on %s: %w", m.resolvedRouteIP, m.resolvedRouteIf, err))
		} else {
			m.resolvedRouteIP = netip.Addr{}
			m.resolvedRouteIf = ""
		}
	}
	if err := m.netConfig.SetDNS(m.s.TunName, nil); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("clear DNS: %w", err))
	}
	if err := m.netConfig.FlushDNS(); err != nil {
		slog.Warn("failed to flush IPv6 DNS cache during cleanup", "err", err)
	}
	if m.tun != nil {
		if err := m.tun.Close(); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("close tun: %w", err))
		}
	}
	return errors.Join(cleanupErrs...)
}
