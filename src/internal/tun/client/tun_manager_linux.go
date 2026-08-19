package client

import (
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"os"
	"strings"
	"tungo/internal/config/client"
	"tungo/internal/config/settings"
	"tungo/internal/platform/command"
	"tungo/internal/tun/internal/linux/epoll"
	"tungo/internal/tun/internal/linux/ioctl"
	"tungo/internal/tun/internal/linux/ip"
	"tungo/internal/tun/internal/linux/mssclamp"
)

type tunWrapper interface {
	Wrap(*os.File) (io.ReadWriteCloser, error)
}

type Manager struct {
	connectionSettings settings.Settings
	configuration      *client.Configuration
	ip                 ip.Contract
	ioctl              ioctl.Contract
	mss                mssclamp.Contract
	wrapper            tunWrapper
	serverAddr         netip.Addr
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
		wrapper:            epoll.NewWrapper(),
	}, nil
}

func (t *Manager) OpenTunnel(serverAddr netip.Addr) (io.ReadWriteCloser, error) {
	if !serverAddr.IsValid() {
		return nil, fmt.Errorf("invalid server address %q", serverAddr)
	}
	serverAddr = serverAddr.Unmap()
	connectionSettings := t.connectionSettings

	// configureTUN client
	if udpConfigurationErr := t.configureTUN(connectionSettings, serverAddr); udpConfigurationErr != nil {
		return nil, fmt.Errorf("failed to configure client: %v", udpConfigurationErr)
	}

	// opens the TUN device
	tunFile, openTunErr := t.ioctl.CreateTunInterface(connectionSettings.TunName)
	if openTunErr != nil {
		return nil, fmt.Errorf("failed to open TUN interface: %v", openTunErr)
	}

	return t.wrapper.Wrap(tunFile)
}

// configureTUN Configures client's TUN device (creates the TUN device, assigns an IP to it, etc)
func (t *Manager) configureTUN(connSettings settings.Settings, serverAddr netip.Addr) error {
	err := t.ip.TunTapAddDevTun(connSettings.TunName)
	if err != nil {
		return err
	}

	err = t.ip.LinkSetDevUp(connSettings.TunName)
	if err != nil {
		return err
	}
	slog.Info("created TUN interface", "name", connSettings.TunName)

	// Assign IPv4 address to the TUN interface
	cidr4, cidr4Err := connSettings.IPv4CIDR()
	if cidr4Err != nil {
		return cidr4Err
	}
	err = t.ip.AddrAddDev(connSettings.TunName, cidr4)
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
		if err := t.ip.AddrAddDev(connSettings.TunName, cidr6); err != nil {
			return err
		}
		slog.Info("assigned IPv6 to interface", "cidr", cidr6, "name", connSettings.TunName)
	}

	serverAddrString := serverAddr.String()

	// Get routing information
	routeInfo, err := t.ip.RouteGet(serverAddrString)
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
		err = t.ip.RouteAddDev(serverAddrString, devInterface)
	} else {
		err = t.ip.RouteAddViaDev(serverAddrString, devInterface, viaGateway)
	}
	if err != nil {
		return fmt.Errorf("failed to add route to server IP: %v", err)
	}
	t.serverAddr = serverAddr
	slog.Info("added route to server", "server_ip", serverAddrString, "via", viaGateway, "device", devInterface)

	// Set split default routes — more specific than 0.0.0.0/0 so they take
	// priority without destroying the original default route. On crash or
	// device deletion the kernel removes them automatically.
	err = t.ip.RouteAddSplitDefaultDev(connSettings.TunName)
	if err != nil {
		return err
	}
	slog.Info("set interface as default gateway for split routes", "name", connSettings.TunName)

	// Set IPv6 split default routes if configured
	if connSettings.IPv6.IsValid() {
		if err := t.ip.Route6AddSplitDefaultDev(connSettings.TunName); err != nil {
			return err
		}
		slog.Info("set interface as IPv6 default gateway for split routes", "name", connSettings.TunName)
	}

	// sets client's TUN device maximum transmission unit (MTU)
	if setMtuErr := t.ip.LinkSetDevMTU(connSettings.TunName, connSettings.MTU); setMtuErr != nil {
		return fmt.Errorf(
			"failed to set %d MTU for %s: %s", connSettings.MTU, connSettings.TunName, setMtuErr,
		)
	}

	// install MSS clamping to prevent PMTU blackholes when forwarding traffic
	if err := t.mss.Install(connSettings.TunName); err != nil {
		return fmt.Errorf("failed to install MSS clamping for %s: %v", connSettings.TunName, err)
	}

	return nil
}

func (t *Manager) CloseTunnel() error {
	t.disposeDevice(t.configuration.TCPSettings)
	t.disposeDevice(t.configuration.UDPSettings)
	t.disposeDevice(t.configuration.WSSettings)
	if t.serverAddr.IsValid() {
		if err := t.ip.RouteDel(t.serverAddr.String()); err != nil {
			return fmt.Errorf("delete route to server %s: %w", t.serverAddr, err)
		}
		t.serverAddr = netip.Addr{}
	}
	return nil
}

func (t *Manager) disposeDevice(s settings.Settings) {
	if s.TunName == "" {
		return
	}
	if err := t.mss.Remove(s.TunName); err != nil {
		slog.Warn("failed to remove MSS clamping", "name", s.TunName, "err", err)
	}
	// Remove split routes before deleting the device
	_ = t.ip.RouteDelSplitDefault(s.TunName)
	_ = t.ip.Route6DelSplitDefault(s.TunName)
	_ = t.ip.LinkDelete(s.TunName)
}
