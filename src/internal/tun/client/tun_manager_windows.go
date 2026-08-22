//go:build windows

package client

import (
	"errors"
	"fmt"
	"io"
	"net/netip"
	"strconv"
	"strings"

	"tungo/internal/config/client"
	"tungo/internal/config/settings"
	"tungo/internal/tun/internal/windows/ipcfg"
	"tungo/internal/tun/internal/windows/wtun"

	"golang.zx2c4.com/wintun"
)

const windowsTunnelType = "TunGo"

type networkConfigurator interface {
	FlushDNS() error
	SetAddressStatic(ifName string, prefix netip.Prefix) error
	SetDNS(ifName string, dnsServers []string) error
	SetMTU(ifName string, mtu int) error
	AddHostRouteViaGateway(hostIP netip.Addr, ifName string, gateway netip.Addr) error
	AddHostRouteOnLink(hostIP netip.Addr, ifName string) error
	AddDefaultSplitRoutes(ifName string) error
	DeleteDefaultSplitRoutes(ifName string) error
	DeleteRoute(destination netip.Addr) error
	DeleteRouteOnInterface(destination netip.Addr, ifName string) error
	BestRoute(dest netip.Addr) (netip.Addr, string, int, int, error)
}

type Manager struct {
	configuration    *client.Configuration
	settings         settings.Settings
	tun              io.ReadWriteCloser
	netConfig4       networkConfigurator
	netConfig6       networkConfigurator
	pinnedServerAddr netip.Addr
	pinnedServerIf   string
}

func New(configuration *client.Configuration) (*Manager, error) {
	active, err := configuration.ActiveSettings()
	if err != nil {
		return nil, err
	}
	return &Manager{
		configuration: configuration,
		settings:      active,
		netConfig4:    ipcfg.NewV4(),
		netConfig6:    ipcfg.NewV6(),
	}, nil
}

func (m *Manager) OpenTunnel(serverAddr netip.Addr) (io.ReadWriter, error) {
	if !serverAddr.IsValid() {
		return nil, fmt.Errorf("invalid server address %q", serverAddr)
	}
	serverAddr = serverAddr.Unmap()
	tun, err := createWindowsTun(m.settings.TunName)
	if err != nil {
		return nil, err
	}
	m.tun = tun
	if err := m.pinServerRoute(serverAddr); err != nil {
		return nil, errors.Join(err, m.closeActiveTunnel())
	}
	if err := m.assignAddresses(); err != nil {
		return nil, errors.Join(err, m.closeActiveTunnel())
	}
	if err := m.addSplitRoutes(); err != nil {
		return nil, errors.Join(err, m.closeActiveTunnel())
	}
	if err := m.setMTU(); err != nil {
		return nil, errors.Join(err, m.closeActiveTunnel())
	}
	if err := m.setDNS(); err != nil {
		return nil, errors.Join(err, m.closeActiveTunnel())
	}
	return m.tun, nil
}

func createWindowsTun(ifName string) (io.ReadWriteCloser, error) {
	adapter, err := wintun.CreateAdapter(ifName, windowsTunnelType, nil)
	if err != nil {
		existing, openErr := wintun.OpenAdapter(ifName)
		if openErr != nil {
			return nil, fmt.Errorf("create/open adapter: %w", err)
		}
		return wtun.NewTUN(existing)
	}
	tun, err := wtun.NewTUN(adapter)
	if err != nil {
		_ = adapter.Close()
		return nil, err
	}
	return tun, nil
}

func (m *Manager) pinServerRoute(serverAddr netip.Addr) error {
	netConfig := m.configuratorFor(serverAddr)
	if netConfig == nil {
		return nil
	}
	gateway, ifName, ifIndex, _, err := netConfig.BestRoute(serverAddr)
	if err != nil {
		return err
	}
	ifName, err = routeInterfaceName(ifName, ifIndex)
	if err != nil {
		return err
	}
	_ = netConfig.DeleteRoute(serverAddr)
	_ = netConfig.DeleteRouteOnInterface(serverAddr, ifName)
	if gateway.IsValid() {
		err = netConfig.AddHostRouteViaGateway(serverAddr, ifName, gateway)
	} else {
		err = netConfig.AddHostRouteOnLink(serverAddr, ifName)
	}
	if err != nil {
		return err
	}
	m.pinnedServerAddr = serverAddr
	m.pinnedServerIf = ifName
	return nil
}

func (m *Manager) configuratorFor(addr netip.Addr) networkConfigurator {
	switch {
	case addr.Is4() && hasIPv4(m.settings):
		return m.netConfig4
	case addr.Is6() && hasIPv6(m.settings):
		return m.netConfig6
	default:
		return nil
	}
}

func (m *Manager) assignAddresses() error {
	if hasIPv4(m.settings) {
		prefix := netip.PrefixFrom(m.settings.IPv4, m.settings.IPv4Subnet.Bits())
		if err := m.netConfig4.SetAddressStatic(m.settings.TunName, prefix); err != nil {
			return fmt.Errorf("set IPv4 address: %w", err)
		}
	}
	if hasIPv6(m.settings) {
		prefix := netip.PrefixFrom(m.settings.IPv6, m.settings.IPv6Subnet.Bits())
		if err := m.netConfig6.SetAddressStatic(m.settings.TunName, prefix); err != nil {
			return fmt.Errorf("set IPv6 address: %w", err)
		}
	}
	return nil
}

func (m *Manager) addSplitRoutes() error {
	if hasIPv4(m.settings) {
		_ = m.netConfig4.DeleteDefaultSplitRoutes(m.settings.TunName)
		if err := m.netConfig4.AddDefaultSplitRoutes(m.settings.TunName); err != nil {
			return fmt.Errorf("add IPv4 split routes: %w", err)
		}
	}
	if hasIPv6(m.settings) {
		_ = m.netConfig6.DeleteDefaultSplitRoutes(m.settings.TunName)
		if err := m.netConfig6.AddDefaultSplitRoutes(m.settings.TunName); err != nil {
			return fmt.Errorf("add IPv6 split routes: %w", err)
		}
	}
	return nil
}

func (m *Manager) setMTU() error {
	if hasIPv4(m.settings) {
		if err := m.netConfig4.SetMTU(m.settings.TunName, m.settings.MTU); err != nil {
			return fmt.Errorf("set IPv4 MTU: %w", err)
		}
	}
	if hasIPv6(m.settings) {
		if err := m.netConfig6.SetMTU(m.settings.TunName, m.settings.MTU); err != nil {
			return fmt.Errorf("set IPv6 MTU: %w", err)
		}
	}
	return nil
}

func (m *Manager) setDNS() error {
	if hasIPv4(m.settings) {
		if err := m.netConfig4.SetDNS(m.settings.TunName, m.settings.DNSv4Resolvers()); err != nil {
			return fmt.Errorf("set IPv4 DNS: %w", err)
		}
	}
	if hasIPv6(m.settings) {
		if err := m.netConfig6.SetDNS(m.settings.TunName, m.settings.DNSv6Resolvers()); err != nil {
			return fmt.Errorf("set IPv6 DNS: %w", err)
		}
	}
	// Resolver cache flushing is best-effort; the configured DNS servers are
	// already active and a flush failure must not tear the tunnel down.
	_ = m.flushDNS(m.settings)
	return nil
}

func (m *Manager) CloseTunnel() error {
	cleanupErrs := []error{m.closeActiveTunnel()}
	activeTunName := m.settings.TunName
	for _, stale := range []settings.Settings{
		m.configuration.TCPSettings,
		m.configuration.UDPSettings,
		m.configuration.WSSettings,
	} {
		if stale.TunName == "" || stale.TunName == activeTunName {
			continue
		}
		if !hasIPv4(stale) && !hasIPv6(stale) {
			continue
		}
		if err := errors.Join(m.cleanupSettings(stale)...); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("clean stale TUN %s: %w", stale.TunName, err))
		}
	}
	return errors.Join(cleanupErrs...)
}

func (m *Manager) closeActiveTunnel() error {
	cleanupErrs := m.cleanupSettings(m.settings)
	if m.pinnedServerAddr.IsValid() {
		netConfig := m.netConfig6
		if m.pinnedServerAddr.Is4() {
			netConfig = m.netConfig4
		}
		if err := netConfig.DeleteRouteOnInterface(m.pinnedServerAddr, m.pinnedServerIf); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("delete route %s on %s: %w", m.pinnedServerAddr, m.pinnedServerIf, err))
		} else {
			m.pinnedServerAddr = netip.Addr{}
			m.pinnedServerIf = ""
		}
	}
	if m.tun != nil {
		tun := m.tun
		m.tun = nil
		if err := tun.Close(); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("close TUN: %w", err))
		}
	}
	return errors.Join(cleanupErrs...)
}

func (m *Manager) cleanupSettings(active settings.Settings) []error {
	var cleanupErrs []error
	if hasIPv4(active) {
		if err := m.netConfig4.DeleteDefaultSplitRoutes(active.TunName); err != nil && !errors.Is(err, ipcfg.ErrInterfaceNotFound) {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("delete IPv4 split routes: %w", err))
		}
		if err := m.netConfig4.SetDNS(active.TunName, nil); err != nil && !errors.Is(err, ipcfg.ErrInterfaceNotFound) {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("clear IPv4 DNS: %w", err))
		}
	}
	if hasIPv6(active) {
		if err := m.netConfig6.DeleteDefaultSplitRoutes(active.TunName); err != nil && !errors.Is(err, ipcfg.ErrInterfaceNotFound) {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("delete IPv6 split routes: %w", err))
		}
		if err := m.netConfig6.SetDNS(active.TunName, nil); err != nil && !errors.Is(err, ipcfg.ErrInterfaceNotFound) {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("clear IPv6 DNS: %w", err))
		}
	}
	if err := m.flushDNS(active); err != nil {
		cleanupErrs = append(cleanupErrs, err)
	}
	return cleanupErrs
}

func (m *Manager) flushDNS(active settings.Settings) error {
	var flushErrs []error
	if hasIPv4(active) {
		if err := m.netConfig4.FlushDNS(); err != nil {
			flushErrs = append(flushErrs, fmt.Errorf("flush IPv4 DNS cache: %w", err))
		}
	}
	if hasIPv6(active) {
		if err := m.netConfig6.FlushDNS(); err != nil {
			flushErrs = append(flushErrs, fmt.Errorf("flush IPv6 DNS cache: %w", err))
		}
	}
	return errors.Join(flushErrs...)
}

func routeInterfaceName(ifName string, ifIndex int) (string, error) {
	trimmed := strings.TrimSpace(ifName)
	if trimmed != "" {
		return trimmed, nil
	}
	if ifIndex <= 0 {
		return "", fmt.Errorf("best route returned empty interface name and invalid index %d", ifIndex)
	}
	return strconv.Itoa(ifIndex), nil
}
