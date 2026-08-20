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

// v4Manager configures a Wintun adapter and the host stack for IPv4.
type v4Manager struct {
	s               settings.Settings
	tun             io.ReadWriteCloser
	netCfg          networkConfigurator
	resolvedRouteIP netip.Addr
	resolvedRouteIf string // cached egress interface used for host route
}

func newV4Manager(
	s settings.Settings,
	netCfg networkConfigurator,
) *v4Manager {
	return &v4Manager{
		s:      s,
		netCfg: netCfg,
	}
}

// OpenTunnel creates/configures the TUN adapter and system netCfgs/DNS for IPv4.
// Safe order: create adapter → host netCfg to server → assign IP → split default → MTU → DNS.
// On any error after adapter creation we call CloseTunnel() to leave the host clean.
func (m *v4Manager) OpenTunnel(serverAddr netip.Addr) (io.ReadWriteCloser, error) {
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
	if err = m.assignIPToTunDevice(); err != nil {
		_ = m.CloseTunnel()
		return nil, err
	}
	if err = m.setDefaultRouteToTunDevice(); err != nil {
		_ = m.CloseTunnel()
		return nil, err
	}
	if err = m.netCfg.SetMTU(m.s.TunName, m.s.MTU); err != nil {
		_ = m.CloseTunnel()
		return nil, err
	}
	if err = m.setDNSToTunDevice(); err != nil {
		_ = m.CloseTunnel()
		return nil, err
	}
	return m.tun, nil
}

// createOrOpenTunDevice creates or opening existing wintun adapter (idempotent behavior).
func (m *v4Manager) createOrOpenTunDevice() (io.ReadWriteCloser, error) {
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

func (m *v4Manager) addStaticRouteToServer(serverAddr netip.Addr) error {
	if !serverAddr.Is4() {
		return nil
	}
	var err error
	gw, ifName, ifIndex, _, bestErr := m.netCfg.BestRoute(serverAddr)
	if bestErr != nil {
		return bestErr
	}
	ifName, err = routeInterfaceName(ifName, ifIndex)
	if err != nil {
		return err
	}
	_ = m.netCfg.DeleteRoute(serverAddr)
	_ = m.netCfg.DeleteRouteOnInterface(serverAddr, ifName)
	var addErr error
	if !gw.IsValid() {
		// on-link
		addErr = m.netCfg.AddHostRouteOnLink(serverAddr, ifName)
	} else {
		addErr = m.netCfg.AddHostRouteViaGateway(serverAddr, ifName, gw)
	}
	if addErr != nil {
		return addErr
	}
	m.resolvedRouteIP = serverAddr
	m.resolvedRouteIf = ifName
	return nil
}

func (m *v4Manager) assignIPToTunDevice() error {
	prefix := netip.PrefixFrom(m.s.IPv4, m.s.IPv4Subnet.Bits())
	if err := m.netCfg.SetAddressStatic(m.s.TunName, prefix); err != nil {
		return err
	}
	return nil
}

// setDefaultRouteToTunDevice replaces any existing default route with split default route (0.0.0.0/1, 128.0.0.0/1).
func (m *v4Manager) setDefaultRouteToTunDevice() error {
	_ = m.netCfg.DeleteDefaultSplitRoutes(m.s.TunName)
	return m.netCfg.AddDefaultSplitRoutes(m.s.TunName)
}

// setDNSToTunDevice applies v4 DNS resolvers and flushes system cache.
func (m *v4Manager) setDNSToTunDevice() error {
	if err := m.netCfg.SetDNS(m.s.TunName, m.s.DNSv4Resolvers()); err != nil {
		return err
	}
	if err := m.netCfg.FlushDNS(); err != nil {
		slog.Warn("failed to flush IPv4 DNS cache", "err", err)
	}
	return nil
}

// CloseTunnel reverses OpenTunnel in safe order.
func (m *v4Manager) CloseTunnel() error {
	var cleanupErrs []error
	if err := m.netCfg.DeleteDefaultSplitRoutes(m.s.TunName); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("delete default split routes: %w", err))
	}
	if m.resolvedRouteIP.IsValid() {
		if err := m.netCfg.DeleteRouteOnInterface(m.resolvedRouteIP, m.resolvedRouteIf); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("delete route %s on %s: %w", m.resolvedRouteIP, m.resolvedRouteIf, err))
		} else {
			m.resolvedRouteIP = netip.Addr{}
			m.resolvedRouteIf = ""
		}
	}
	if err := m.netCfg.SetDNS(m.s.TunName, nil); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("clear DNS: %w", err))
	}
	if err := m.netCfg.FlushDNS(); err != nil {
		slog.Warn("failed to flush IPv4 DNS cache during cleanup", "err", err)
	}
	if m.tun != nil {
		if err := m.tun.Close(); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("close tun: %w", err))
		}
	}
	return errors.Join(cleanupErrs...)
}
