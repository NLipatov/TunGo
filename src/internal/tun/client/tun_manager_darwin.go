//go:build darwin

package client

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"

	"tungo/internal/config/client"
	"tungo/internal/config/settings"
	"tungo/internal/platform/command"
	"tungo/internal/tun/internal/darwin/dns"
	"tungo/internal/tun/internal/darwin/ifconfig"
	"tungo/internal/tun/internal/darwin/route"
	"tungo/internal/tun/internal/darwin/utun"
)

type tun interface {
	io.ReadWriteCloser
	Name() string
}

type interfaceConfigurator interface {
	LinkAddrAdd(ifName string, prefix netip.Prefix) error
	SetMTU(ifName string, mtu int) error
}

type routeConfigurator interface {
	Add(destIP string) error
	AddSplit(ifName string) error
	DelSplit(ifName string) error
	Del(destIP string) error
}

type dnsConfigurator interface {
	Set(resolvers []string) error
	Revert() error
}

type Manager struct {
	settings         settings.Settings
	tun              tun
	dns              dnsConfigurator
	ifconfig4        interfaceConfigurator
	ifconfig6        interfaceConfigurator
	route4           routeConfigurator
	route6           routeConfigurator
	pinnedServerAddr netip.Addr
}

// New creates a tunnel manager from a normalized, validated client configuration.
func New(configuration *client.Configuration) (*Manager, error) {
	active, err := configuration.ActiveSettings()
	if err != nil {
		return nil, err
	}
	cmd := command.New()
	return &Manager{
		settings:  active,
		dns:       dns.New(cmd),
		ifconfig4: ifconfig.NewV4(cmd),
		ifconfig6: ifconfig.NewV6(cmd),
		route4:    route.NewV4(cmd),
		route6:    route.NewV6(cmd),
	}, nil
}

func (m *Manager) OpenTunnel(serverAddr netip.Addr) (io.ReadWriter, error) {
	if !serverAddr.IsValid() {
		return nil, fmt.Errorf("invalid server address %q", serverAddr)
	}
	serverAddr = serverAddr.Unmap()
	tun, err := utun.New()
	if err != nil {
		return nil, fmt.Errorf("create utun: %w", err)
	}
	m.tun = tun
	if err := m.setMTU(); err != nil {
		return nil, errors.Join(err, m.CloseTunnel())
	}
	if err := m.pinServerRoute(serverAddr); err != nil {
		return nil, errors.Join(err, m.CloseTunnel())
	}
	if err := m.assignAddresses(); err != nil {
		return nil, errors.Join(err, m.CloseTunnel())
	}
	if err := m.addSplitRoutes(); err != nil {
		return nil, errors.Join(err, m.CloseTunnel())
	}
	if err := m.setDNS(); err != nil {
		slog.Warn("failed to configure DNS", "interface", m.tun.Name(), "err", err)
	}
	return m.tun, nil
}

func (m *Manager) setDNS() error {
	resolvers := make([]string, 0, 4)
	if m.settings.HasIPv4() {
		resolvers = append(resolvers, m.settings.DNSv4...)
	}
	if m.settings.HasIPv6() {
		resolvers = append(resolvers, m.settings.DNSv6...)
	}

	if err := m.dns.Set(resolvers); err != nil {
		return fmt.Errorf("set DNS: %w", err)
	}
	return nil
}

func (m *Manager) setMTU() error {
	configurator := m.ifconfig4
	if !m.settings.HasIPv4() {
		configurator = m.ifconfig6
	}
	if err := configurator.SetMTU(m.tun.Name(), m.settings.MTU); err != nil {
		return fmt.Errorf("set MTU %d on %s: %w", m.settings.MTU, m.tun.Name(), err)
	}
	return nil
}

func (m *Manager) pinServerRoute(serverAddr netip.Addr) error {
	var route routeConfigurator
	switch {
	case serverAddr.Is4() && m.settings.HasIPv4():
		route = m.route4
	case serverAddr.Is6() && m.settings.HasIPv6():
		route = m.route6
	default:
		return nil
	}
	if err := route.Add(serverAddr.String()); err != nil {
		return fmt.Errorf("route to server %s: %w", serverAddr, err)
	}
	m.pinnedServerAddr = serverAddr
	return nil
}

func (m *Manager) assignAddresses() error {
	if m.settings.HasIPv4() {
		prefix := netip.PrefixFrom(m.settings.IPv4, m.settings.IPv4.BitLen())
		if err := m.ifconfig4.LinkAddrAdd(m.tun.Name(), prefix); err != nil {
			return fmt.Errorf("set IPv4 address %s on %s: %w", prefix, m.tun.Name(), err)
		}
	}
	if m.settings.HasIPv6() {
		prefix := netip.PrefixFrom(m.settings.IPv6, m.settings.IPv6Subnet.Bits())
		if err := m.ifconfig6.LinkAddrAdd(m.tun.Name(), prefix); err != nil {
			return fmt.Errorf("set IPv6 address %s on %s: %w", prefix, m.tun.Name(), err)
		}
	}
	return nil
}

func (m *Manager) addSplitRoutes() error {
	if m.settings.HasIPv4() {
		_ = m.route4.DelSplit(m.tun.Name())
		if err := m.route4.AddSplit(m.tun.Name()); err != nil {
			return fmt.Errorf("add IPv4 split default: %w", err)
		}
	}
	if m.settings.HasIPv6() {
		_ = m.route6.DelSplit(m.tun.Name())
		if err := m.route6.AddSplit(m.tun.Name()); err != nil {
			return fmt.Errorf("add IPv6 split default: %w", err)
		}
	}
	return nil
}

func (m *Manager) CloseTunnel() error {
	var cleanupErrs []error
	if err := m.dns.Revert(); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("restore DNS: %w", err))
	}
	if m.tun != nil {
		if m.settings.HasIPv4() {
			if err := m.route4.DelSplit(m.tun.Name()); err != nil {
				cleanupErrs = append(cleanupErrs, fmt.Errorf("delete IPv4 split routes: %w", err))
			}
		}
		if m.settings.HasIPv6() {
			if err := m.route6.DelSplit(m.tun.Name()); err != nil {
				cleanupErrs = append(cleanupErrs, fmt.Errorf("delete IPv6 split routes: %w", err))
			}
		}
	}
	if m.pinnedServerAddr.IsValid() {
		var err error
		if m.pinnedServerAddr.Is4() {
			err = m.route4.Del(m.pinnedServerAddr.String())
		} else {
			err = m.route6.Del(m.pinnedServerAddr.String())
		}
		if err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("delete route to server %s: %w", m.pinnedServerAddr, err))
		} else {
			m.pinnedServerAddr = netip.Addr{}
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
