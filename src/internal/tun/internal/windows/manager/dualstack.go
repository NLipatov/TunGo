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

// dualStackManager configures one Wintun adapter for both IPv4 and IPv6 stacks.
type dualStackManager struct {
	s   settings.Settings
	tun io.ReadWriteCloser

	netCfg4 networkConfigurator
	netCfg6 networkConfigurator

	resolvedRouteIP4 netip.Addr
	resolvedRouteIP6 netip.Addr
	resolvedRouteIf4 string // cached egress interface for IPv4 host route
	resolvedRouteIf6 string // cached egress interface for IPv6 host route
}

func newDualStackManager(
	s settings.Settings,
	netCfg4, netCfg6 networkConfigurator,
) *dualStackManager {
	return &dualStackManager{
		s:       s,
		netCfg4: netCfg4,
		netCfg6: netCfg6,
	}
}

func (m *dualStackManager) OpenTunnel(serverAddr netip.Addr) (io.ReadWriteCloser, error) {
	if !serverAddr.IsValid() {
		return nil, fmt.Errorf("invalid server address %q", serverAddr)
	}
	serverAddr = serverAddr.Unmap()

	tunDev, err := m.createOrOpenTunDevice()
	if err != nil {
		return nil, err
	}
	m.tun = tunDev

	if err = m.addStaticRouteToServer(serverAddr); err != nil {
		_ = m.CloseTunnel()
		return nil, err
	}
	if err = m.assignIPv4ToTunDevice(); err != nil {
		_ = m.CloseTunnel()
		return nil, err
	}
	if err = m.assignIPv6ToTunDevice(); err != nil {
		_ = m.CloseTunnel()
		return nil, err
	}
	if err = m.setDefaultRoutesToTunDevice(); err != nil {
		_ = m.CloseTunnel()
		return nil, err
	}
	if err = m.setMTUToTunDevice(); err != nil {
		_ = m.CloseTunnel()
		return nil, err
	}
	if err = m.setDNSToTunDevice(); err != nil {
		_ = m.CloseTunnel()
		return nil, err
	}

	return m.tun, nil
}

func (m *dualStackManager) createOrOpenTunDevice() (io.ReadWriteCloser, error) {
	adapter, err := wintun.CreateAdapter(m.s.TunName, tunnelType, nil)
	if err != nil {
		if existing, openErr := wintun.OpenAdapter(m.s.TunName); openErr == nil {
			return wtun.NewTUN(existing)
		}
		return nil, fmt.Errorf("create/open adapter: %w", err)
	}
	tunDev, tunDevErr := wtun.NewTUN(adapter)
	if tunDevErr != nil {
		_ = adapter.Close()
		return nil, tunDevErr
	}
	return tunDev, nil
}

func (m *dualStackManager) addStaticRouteToServer(serverAddr netip.Addr) error {
	if serverAddr.Is4() {
		return m.addStaticRouteToServer4(serverAddr)
	}
	return m.addStaticRouteToServer6(serverAddr)
}

func (m *dualStackManager) addStaticRouteToServer4(routeIP netip.Addr) error {
	var err error
	gw, ifName, ifIndex, _, bestErr := m.netCfg4.BestRoute(routeIP)
	if bestErr != nil {
		return bestErr
	}
	ifName, err = routeInterfaceName(ifName, ifIndex)
	if err != nil {
		return err
	}
	_ = m.netCfg4.DeleteRoute(routeIP)
	_ = m.netCfg4.DeleteRouteOnInterface(routeIP, ifName)
	var addErr error
	if !gw.IsValid() {
		addErr = m.netCfg4.AddHostRouteOnLink(routeIP, ifName)
	} else {
		addErr = m.netCfg4.AddHostRouteViaGateway(routeIP, ifName, gw)
	}
	if addErr != nil {
		return addErr
	}
	m.resolvedRouteIP4 = routeIP
	m.resolvedRouteIf4 = ifName
	return nil
}

func (m *dualStackManager) addStaticRouteToServer6(routeIP netip.Addr) error {
	var err error
	gw, ifName, ifIndex, _, bestErr := m.netCfg6.BestRoute(routeIP)
	if bestErr != nil {
		return bestErr
	}
	ifName, err = routeInterfaceName(ifName, ifIndex)
	if err != nil {
		return err
	}
	_ = m.netCfg6.DeleteRoute(routeIP)
	_ = m.netCfg6.DeleteRouteOnInterface(routeIP, ifName)
	var addErr error
	if !gw.IsValid() {
		addErr = m.netCfg6.AddHostRouteOnLink(routeIP, ifName)
	} else {
		addErr = m.netCfg6.AddHostRouteViaGateway(routeIP, ifName, gw)
	}
	if addErr != nil {
		return addErr
	}
	m.resolvedRouteIP6 = routeIP
	m.resolvedRouteIf6 = ifName
	return nil
}

func (m *dualStackManager) assignIPv4ToTunDevice() error {
	prefix := netip.PrefixFrom(m.s.IPv4, m.s.IPv4Subnet.Bits())
	return m.netCfg4.SetAddressStatic(m.s.TunName, prefix)
}

func (m *dualStackManager) assignIPv6ToTunDevice() error {
	prefix := netip.PrefixFrom(m.s.IPv6, m.s.IPv6Subnet.Bits())
	return m.netCfg6.SetAddressStatic(m.s.TunName, prefix)
}

func (m *dualStackManager) setDefaultRoutesToTunDevice() error {
	_ = m.netCfg4.DeleteDefaultSplitRoutes(m.s.TunName)
	if err := m.netCfg4.AddDefaultSplitRoutes(m.s.TunName); err != nil {
		return err
	}

	_ = m.netCfg6.DeleteDefaultSplitRoutes(m.s.TunName)
	return m.netCfg6.AddDefaultSplitRoutes(m.s.TunName)
}

func (m *dualStackManager) setMTUToTunDevice() error {
	if err := m.netCfg4.SetMTU(m.s.TunName, m.s.MTU); err != nil {
		return fmt.Errorf("set IPv4 MTU: %w", err)
	}
	if err := m.netCfg6.SetMTU(m.s.TunName, m.s.MTU); err != nil {
		return fmt.Errorf("set IPv6 MTU: %w", err)
	}
	return nil
}

func (m *dualStackManager) setDNSToTunDevice() error {
	if err := m.netCfg4.SetDNS(m.s.TunName, m.s.DNSv4Resolvers()); err != nil {
		return err
	}
	if err := m.netCfg6.SetDNS(m.s.TunName, m.s.DNSv6Resolvers()); err != nil {
		return err
	}
	if err := m.netCfg4.FlushDNS(); err != nil {
		slog.Warn("failed to flush IPv4 DNS cache", "err", err)
	}
	if err := m.netCfg6.FlushDNS(); err != nil {
		slog.Warn("failed to flush IPv6 DNS cache", "err", err)
	}
	return nil
}

func (m *dualStackManager) CloseTunnel() error {
	var cleanupErrs []error
	if err := m.netCfg4.DeleteDefaultSplitRoutes(m.s.TunName); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("delete IPv4 split routes: %w", err))
	}
	if err := m.netCfg6.DeleteDefaultSplitRoutes(m.s.TunName); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("delete IPv6 split routes: %w", err))
	}

	if m.resolvedRouteIP4.IsValid() {
		if err := m.netCfg4.DeleteRouteOnInterface(m.resolvedRouteIP4, m.resolvedRouteIf4); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("delete IPv4 route %s on %s: %w", m.resolvedRouteIP4, m.resolvedRouteIf4, err))
		} else {
			m.resolvedRouteIP4 = netip.Addr{}
			m.resolvedRouteIf4 = ""
		}
	}
	if m.resolvedRouteIP6.IsValid() {
		if err := m.netCfg6.DeleteRouteOnInterface(m.resolvedRouteIP6, m.resolvedRouteIf6); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("delete IPv6 route %s on %s: %w", m.resolvedRouteIP6, m.resolvedRouteIf6, err))
		} else {
			m.resolvedRouteIP6 = netip.Addr{}
			m.resolvedRouteIf6 = ""
		}
	}
	// Best-effort DNS cleanup to avoid leaving partial resolver state on rollback.
	if err := m.netCfg4.SetDNS(m.s.TunName, nil); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("clear IPv4 DNS: %w", err))
	}
	if err := m.netCfg6.SetDNS(m.s.TunName, nil); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("clear IPv6 DNS: %w", err))
	}
	if err := m.netCfg4.FlushDNS(); err != nil {
		slog.Warn("failed to flush IPv4 DNS cache during cleanup", "err", err)
	}
	if err := m.netCfg6.FlushDNS(); err != nil {
		slog.Warn("failed to flush IPv6 DNS cache during cleanup", "err", err)
	}

	if m.tun != nil {
		if err := m.tun.Close(); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("close tun: %w", err))
		}
	}
	return errors.Join(cleanupErrs...)
}
