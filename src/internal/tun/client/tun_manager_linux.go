package client

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
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
	connectionSettings settings.Settings
	configuration      *client.Configuration
	ip                 ip.Contract
	ioctl              ioctl.Contract
	mss                mssclamp.Contract
	serverAddr         netip.Addr
	tun                io.ReadWriteCloser
}

func New(conf *client.Configuration) (*Manager, error) {
	connectionSettings, err := conf.ActiveSettings()
	if err != nil {
		return nil, err
	}
	return &Manager{
		connectionSettings: connectionSettings,
		configuration:      conf,
		ip:                 ip.NewWrapper(command.New()),
		ioctl:              ioctl.NewWrapper(ioctl.NewLinuxIoctlCommander(), "/dev/net/tun"),
		mss:                mssclamp.NewManager(command.New()),
	}, nil
}

func (m *Manager) OpenTunnel(serverAddr netip.Addr) (io.ReadWriter, error) {
	if !serverAddr.IsValid() {
		return nil, fmt.Errorf("invalid server address %q", serverAddr)
	}
	serverAddr = serverAddr.Unmap()
	connectionSettings := m.connectionSettings

	// configureTUN client
	if udpConfigurationErr := m.configureTUN(connectionSettings, serverAddr); udpConfigurationErr != nil {
		openErr := fmt.Errorf("failed to configure client: %w", udpConfigurationErr)
		return nil, errors.Join(openErr, m.CloseTunnel())
	}

	// opens the TUN device
	tunFile, openTunErr := m.ioctl.CreateTunInterface(connectionSettings.TunName)
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

// configureTUN Configures client's TUN device (creates the TUN device, assigns an IP to it, etc)
func (m *Manager) configureTUN(connSettings settings.Settings, serverAddr netip.Addr) error {
	err := m.ip.TunTapAddDevTun(connSettings.TunName)
	if err != nil {
		return err
	}

	err = m.ip.LinkSetDevUp(connSettings.TunName)
	if err != nil {
		return err
	}
	slog.Info("created TUN interface", "name", connSettings.TunName)

	// Assign IPv4 address to the TUN interface
	cidr4, cidr4Err := connSettings.IPv4CIDR()
	if cidr4Err != nil {
		return cidr4Err
	}
	err = m.ip.AddrAddDev(connSettings.TunName, cidr4)
	if err != nil {
		return err
	}
	slog.Info("assigned IPv4 to interface", "cidr", cidr4, "name", connSettings.TunName)

	// Assign IPv6 address if configured
	if connSettings.IPv6.IsValid() && connSettings.IPv6Subnet.IsValid() {
		cidr6, cidr6Err := connSettings.IPv6CIDR()
		if cidr6Err != nil {
			return cidr6Err
		}
		if err := m.ip.AddrAddDev(connSettings.TunName, cidr6); err != nil {
			return err
		}
		slog.Info("assigned IPv6 to interface", "cidr", cidr6, "name", connSettings.TunName)
	}

	serverAddrString := serverAddr.String()

	// Get routing information
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

	// Add route to server IP
	if viaGateway == "" {
		err = m.ip.RouteAddDev(serverAddrString, devInterface)
	} else {
		err = m.ip.RouteAddViaDev(serverAddrString, devInterface, viaGateway)
	}
	if err != nil {
		return fmt.Errorf("failed to add route to server IP: %v", err)
	}
	m.serverAddr = serverAddr
	slog.Info("added route to server", "server_ip", serverAddrString, "via", viaGateway, "device", devInterface)

	// Set split default routes — more specific than 0.0.0.0/0 so they take
	// priority without destroying the original default route. On crash or
	// device deletion the kernel removes them automatically.
	err = m.ip.RouteAddSplitDefaultDev(connSettings.TunName)
	if err != nil {
		return err
	}
	slog.Info("set interface as default gateway for split routes", "name", connSettings.TunName)

	// Set IPv6 split default routes if configured
	if connSettings.IPv6.IsValid() {
		if err := m.ip.Route6AddSplitDefaultDev(connSettings.TunName); err != nil {
			return err
		}
		slog.Info("set interface as IPv6 default gateway for split routes", "name", connSettings.TunName)
	}

	// sets client's TUN device maximum transmission unit (MTU)
	if setMtuErr := m.ip.LinkSetDevMTU(connSettings.TunName, connSettings.MTU); setMtuErr != nil {
		return fmt.Errorf(
			"failed to set %d MTU for %s: %s", connSettings.MTU, connSettings.TunName, setMtuErr,
		)
	}

	// install MSS clamping to prevent PMTU blackholes when forwarding traffic
	if err := m.mss.Install(connSettings.TunName); err != nil {
		return fmt.Errorf("failed to install MSS clamping for %s: %v", connSettings.TunName, err)
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

	m.removeTunInterface(m.configuration.TCPSettings)
	m.removeTunInterface(m.configuration.UDPSettings)
	m.removeTunInterface(m.configuration.WSSettings)
	if m.serverAddr.IsValid() {
		if err := m.ip.RouteDel(m.serverAddr.String()); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("delete route to server %s: %w", m.serverAddr, err))
		} else {
			m.serverAddr = netip.Addr{}
		}
	}
	return errors.Join(cleanupErrs...)
}

func (m *Manager) removeTunInterface(s settings.Settings) {
	if s.TunName == "" {
		return
	}
	if err := m.mss.Remove(s.TunName); err != nil {
		slog.Warn("failed to remove MSS clamping", "name", s.TunName, "err", err)
	}
	// Remove split routes before deleting the device
	_ = m.ip.RouteDelSplitDefault(s.TunName)
	_ = m.ip.Route6DelSplitDefault(s.TunName)
	_ = m.ip.LinkDelete(s.TunName)
}
