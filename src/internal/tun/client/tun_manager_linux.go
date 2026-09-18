package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"strings"

	"tungo/internal/config/client"
	"tungo/internal/config/settings"
	"tungo/internal/platform/command"
	"tungo/internal/tun/internal/linux/defaultroute"
	"tungo/internal/tun/internal/linux/dns"
	"tungo/internal/tun/internal/linux/epoll"
	"tungo/internal/tun/internal/linux/ioctl"
	"tungo/internal/tun/internal/linux/ip"
	"tungo/internal/tun/internal/linux/mssclamp"
)

type dnsConfigurator interface {
	Set(ifName string, ipv4Resolvers, ipv6Resolvers []string) error
	Revert() error
}

type Manager struct {
	configuration             *client.Configuration
	settings                  settings.Settings
	dns                       dnsConfigurator
	ip                        ip.Contract
	ioctl                     ioctl.Contract
	mss                       mssclamp.Contract
	pinnedServerAddr          netip.Addr
	tun                       io.ReadWriteCloser
	defaultRouteWatcherCancel context.CancelFunc
	splitsv4                  []string
	splitsv6                  []string
}

// New creates a tunnel manager from a normalized, validated client configuration.
func New(conf *client.Configuration) (*Manager, error) {
	active, err := conf.ActiveSettings()
	if err != nil {
		return nil, err
	}
	cmd := command.New()
	return &Manager{
		configuration: conf,
		settings:      active,
		dns:           dns.New(cmd),
		ip:            ip.New(cmd),
		ioctl:         ioctl.New(ioctl.NewLinuxIoctlCommander(), "/dev/net/tun"),
		mss:           mssclamp.NewManager(cmd),
		splitsv4:      withoutTunSubnet(conf.AllowedIPsv4, active.IPv4Subnet.Masked().String()),
		splitsv6:      withoutTunSubnet(conf.AllowedIPsv6, active.IPv6Subnet.Masked().String()),
	}, nil
}

func (m *Manager) OpenTunnel(serverAddr netip.Addr) (io.ReadWriter, error) {
	if !serverAddr.IsValid() {
		return nil, fmt.Errorf("invalid server address %q", serverAddr)
	}
	serverAddr = serverAddr.Unmap()
	tunFile, openTunErr := m.ioctl.CreateTunInterface(m.settings.TunName)
	if openTunErr != nil {
		openErr := fmt.Errorf("failed to open TUN interface: %w", openTunErr)
		return nil, errors.Join(openErr, m.CloseTunnel())
	}
	tun, err := epoll.New(tunFile)
	if err != nil {
		openErr := fmt.Errorf("failed to initialize TUN I/O: %w", err)
		return nil, errors.Join(openErr, tunFile.Close(), m.CloseTunnel())
	}
	if err := m.watchDefaultRoute(tun); err != nil {
		slog.Warn("failed to configure default route watcher", "err", err)
	}
	m.tun = tun
	if err := m.configureTunnel(serverAddr); err != nil {
		openErr := fmt.Errorf("failed to configure client: %w", err)
		return nil, errors.Join(openErr, m.CloseTunnel())
	}
	if err := m.setDNS(); err != nil {
		slog.Warn("failed to configure DNS", "interface", m.settings.TunName, "err", err)
	}
	return m.tun, nil
}

func (m *Manager) watchDefaultRoute(tun io.Closer) error {
	ctx, cancel := context.WithCancel(context.Background())
	errCh, err := defaultroute.Watch(ctx)
	if err != nil {
		cancel()
		return err
	}
	go func() {
		select {
		case <-ctx.Done():
			return
		case err := <-errCh:
			if err != nil {
				slog.Warn("default route watcher failed", "err", err)
			} else {
				slog.Info("default route change detected")
			}
			_ = tun.Close()
		}
	}()
	m.defaultRouteWatcherCancel = cancel
	return nil
}

func (m *Manager) configureTunnel(serverAddr netip.Addr) error {
	err := m.ip.LinkSetDevUp(m.settings.TunName)
	if err != nil {
		return err
	}
	if m.settings.HasIPv4() {
		cidr4, cidr4Err := m.settings.IPv4CIDR()
		if cidr4Err != nil {
			return cidr4Err
		}
		if err := m.ip.AddrAddDev(m.settings.TunName, cidr4); err != nil {
			return err
		}
	}

	if m.settings.HasIPv6() {
		cidr6, cidr6Err := m.settings.IPv6CIDR()
		if cidr6Err != nil {
			return cidr6Err
		}
		if err := m.ip.AddrAddDev(m.settings.TunName, cidr6); err != nil {
			return err
		}
	}

	if serverAddr.Is4() && m.settings.HasIPv4() || serverAddr.Is6() && m.settings.HasIPv6() {
		routeInfo, err := m.ip.RouteGet(serverAddr)
		if err != nil {
			return err
		}
		var viaGateway, devInterface string
		fields := strings.Fields(routeInfo)
		for i, field := range fields {
			if field == "via" && i+1 < len(fields) {
				viaGateway = fields[i+1]
			}
			if field == "dev" && i+1 < len(fields) {
				devInterface = fields[i+1]
			}
		}
		if devInterface == "" {
			return fmt.Errorf("failed to parse route to server IP")
		}
		if viaGateway == "" {
			err = m.ip.RouteReplaceDev(serverAddr, devInterface)
		} else {
			gateway, parseErr := netip.ParseAddr(viaGateway)
			if parseErr != nil {
				return fmt.Errorf("failed to parse route gateway %q: %w", viaGateway, parseErr)
			}
			err = m.ip.RouteReplaceViaDev(serverAddr, devInterface, gateway)
		}
		if err != nil {
			return fmt.Errorf("failed to replace route to server IP: %v", err)
		}
		m.pinnedServerAddr = serverAddr
	}

	if m.settings.HasIPv4() {
		if err := m.ip.RouteAddSplitDefaultDev(m.settings.TunName, m.splitsv4); err != nil {
			return err
		}
	}

	if m.settings.HasIPv6() {
		if err := m.ip.Route6AddSplitDefaultDev(m.settings.TunName, m.splitsv6); err != nil {
			return err
		}
	}

	if setMtuErr := m.ip.LinkSetDevMTU(m.settings.TunName, m.settings.MTU); setMtuErr != nil {
		return fmt.Errorf(
			"failed to set %d MTU for %s: %s", m.settings.MTU, m.settings.TunName, setMtuErr,
		)
	}

	families := mssclamp.Families{
		IPv4: m.settings.HasIPv4(),
		IPv6: m.settings.HasIPv6(),
	}
	if err := m.mss.Install(m.settings.TunName, families); err != nil {
		return fmt.Errorf("failed to install MSS clamping for %s: %v", m.settings.TunName, err)
	}

	return nil
}

func (m *Manager) setDNS() error {
	var ipv4Resolvers, ipv6Resolvers []string
	if m.settings.HasIPv4() {
		ipv4Resolvers = m.settings.DNSv4
	}
	if m.settings.HasIPv6() {
		ipv6Resolvers = m.settings.DNSv6
	}

	if err := m.dns.Set(m.settings.TunName, ipv4Resolvers, ipv6Resolvers); err != nil {
		return fmt.Errorf("set DNS on %s: %w", m.settings.TunName, err)
	}
	return nil
}

func (m *Manager) CloseTunnel() error {
	if m.defaultRouteWatcherCancel != nil {
		m.defaultRouteWatcherCancel()
	}
	var cleanupErrs []error
	if err := m.dns.Revert(); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("restore DNS: %w", err))
	}
	if m.tun != nil {
		if err := m.tun.Close(); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("close TUN: %w", err))
		}
		m.tun = nil
	}

	cleanupErrs = append(cleanupErrs,
		m.removeTunInterface(m.configuration.TCPSettings),
		m.removeTunInterface(m.configuration.UDPSettings),
		m.removeTunInterface(m.configuration.WSSettings),
	)
	if m.pinnedServerAddr.IsValid() {
		if err := m.ip.RouteDel(m.pinnedServerAddr); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("delete route to server %s: %w", m.pinnedServerAddr, err))
		} else {
			m.pinnedServerAddr = netip.Addr{}
		}
	}
	return errors.Join(cleanupErrs...)
}

func (m *Manager) removeTunInterface(s settings.Settings) error {
	if s.TunName == "" {
		return nil
	}
	var errs []error
	if err := m.mss.Remove(s.TunName); err != nil {
		errs = append(errs, fmt.Errorf("remove MSS clamping for %s: %w", s.TunName, err))
	}
	if err := m.ip.LinkDelete(s.TunName); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
