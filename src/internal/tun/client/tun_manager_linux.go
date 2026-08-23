package client

import (
	"errors"
	"fmt"
	"io"
	"net/netip"
	"strings"
	"tungo/internal/config/client"
	"tungo/internal/config/settings"
	"tungo/internal/platform/command"
	"tungo/internal/tun/internal/linux/epoll"
	"tungo/internal/tun/internal/linux/ioctl"
	"tungo/internal/tun/internal/linux/ip"
	"tungo/internal/tun/internal/linux/mssclamp"
)

type Manager struct {
	configuration    *client.Configuration
	settings         settings.Settings
	ip               ip.Contract
	ioctl            ioctl.Contract
	mss              mssclamp.Contract
	pinnedServerAddr netip.Addr
	tun              io.ReadWriteCloser
}

func New(conf *client.Configuration) (*Manager, error) {
	active, err := conf.ActiveSettings()
	if err != nil {
		return nil, err
	}
	return &Manager{
		configuration: conf,
		settings:      active,
		ip:            ip.New(command.New()),
		ioctl:         ioctl.New(ioctl.NewLinuxIoctlCommander(), "/dev/net/tun"),
		mss:           mssclamp.NewManager(command.New()),
	}, nil
}

func (m *Manager) OpenTunnel(serverAddr netip.Addr) (io.ReadWriter, error) {
	if !serverAddr.IsValid() {
		return nil, fmt.Errorf("invalid server address %q", serverAddr)
	}
	serverAddr = serverAddr.Unmap()

	if err := m.configureTunnel(serverAddr); err != nil {
		openErr := fmt.Errorf("failed to configure client: %w", err)
		return nil, errors.Join(openErr, m.CloseTunnel())
	}

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
	m.tun = tun
	return m.tun, nil
}

func (m *Manager) configureTunnel(serverAddr netip.Addr) error {
	err := m.ip.TunTapAddDevTun(m.settings.TunName)
	if err != nil {
		return err
	}

	err = m.ip.LinkSetDevUp(m.settings.TunName)
	if err != nil {
		return err
	}
	if hasIPv4(m.settings) {
		cidr4, cidr4Err := m.settings.IPv4CIDR()
		if cidr4Err != nil {
			return cidr4Err
		}
		if err := m.ip.AddrAddDev(m.settings.TunName, cidr4); err != nil {
			return err
		}
	}

	if hasIPv6(m.settings) {
		cidr6, cidr6Err := m.settings.IPv6CIDR()
		if cidr6Err != nil {
			return cidr6Err
		}
		if err := m.ip.AddrAddDev(m.settings.TunName, cidr6); err != nil {
			return err
		}
	}

	if serverAddr.Is4() && hasIPv4(m.settings) || serverAddr.Is6() && hasIPv6(m.settings) {
		serverAddrString := serverAddr.String()
		routeInfo, err := m.ip.RouteGet(serverAddrString)
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
			err = m.ip.RouteReplaceDev(serverAddrString, devInterface)
		} else {
			err = m.ip.RouteReplaceViaDev(serverAddrString, devInterface, viaGateway)
		}
		if err != nil {
			return fmt.Errorf("failed to replace route to server IP: %v", err)
		}
		m.pinnedServerAddr = serverAddr
	}

	// Set split default routes — more specific than 0.0.0.0/0 so they take
	// priority without destroying the original default route. On crash or
	// device deletion the kernel removes them automatically.
	if hasIPv4(m.settings) {
		if err := m.ip.RouteAddSplitDefaultDev(m.settings.TunName); err != nil {
			return err
		}
	}

	if hasIPv6(m.settings) {
		if err := m.ip.Route6AddSplitDefaultDev(m.settings.TunName); err != nil {
			return err
		}
	}

	if setMtuErr := m.ip.LinkSetDevMTU(m.settings.TunName, m.settings.MTU); setMtuErr != nil {
		return fmt.Errorf(
			"failed to set %d MTU for %s: %s", m.settings.MTU, m.settings.TunName, setMtuErr,
		)
	}

	families := mssclamp.Families{
		IPv4: hasIPv4(m.settings),
		IPv6: hasIPv6(m.settings),
	}
	if err := m.mss.Install(m.settings.TunName, families); err != nil {
		return fmt.Errorf("failed to install MSS clamping for %s: %v", m.settings.TunName, err)
	}

	return nil
}

func (m *Manager) CloseTunnel() error {
	var cleanupErrs []error
	if m.tun != nil {
		tun := m.tun
		m.tun = nil
		if err := tun.Close(); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("close TUN: %w", err))
		}
	}

	cleanupErrs = append(cleanupErrs,
		m.removeTunInterface(m.configuration.TCPSettings),
		m.removeTunInterface(m.configuration.UDPSettings),
		m.removeTunInterface(m.configuration.WSSettings),
	)
	if m.pinnedServerAddr.IsValid() {
		if err := m.ip.RouteDel(m.pinnedServerAddr.String()); err != nil {
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
	var cleanupErrs []error
	if err := m.mss.Remove(s.TunName); err != nil {
		cleanupErrs = append(cleanupErrs, fmt.Errorf("remove MSS clamping for %s: %w", s.TunName, err))
	}
	if hasIPv4(s) {
		_ = m.ip.RouteDelSplitDefault(s.TunName)
	}
	if hasIPv6(s) {
		_ = m.ip.Route6DelSplitDefault(s.TunName)
	}
	_ = m.ip.LinkDelete(s.TunName)
	return errors.Join(cleanupErrs...)
}
