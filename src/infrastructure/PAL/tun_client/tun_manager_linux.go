package tun_client

import (
	"fmt"
	"log"
	"net/netip"
	"strings"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/exec_commander"
	"tungo/infrastructure/PAL/linux/network_tools/ioctl"
	"tungo/infrastructure/PAL/linux/network_tools/ip"
	"tungo/infrastructure/PAL/linux/network_tools/mssclamp"
	"tungo/infrastructure/PAL/linux/tun/epoll"
	"tungo/infrastructure/settings"
)

// PlatformTunManager Linux-specific TunDevice manager
type PlatformTunManager struct {
	configuration client.Configuration
	ip            ip.Contract
	ioctl         ioctl.Contract
	mss           mssclamp.Contract
	wrapper       tun.Wrapper
}

func NewPlatformTunManager(
	configuration client.Configuration,
) (tun.ClientManager, error) {
	return &PlatformTunManager{
		configuration: configuration,
		ip:            ip.NewWrapper(exec_commander.NewExecCommander()),
		ioctl:         ioctl.NewWrapper(ioctl.NewLinuxIoctlCommander(), "/dev/net/tun"),
		mss:           mssclamp.NewManager(exec_commander.NewExecCommander()),
		wrapper:       epoll.NewWrapper(),
	}, nil
}

func (t *PlatformTunManager) CreateDevice() (tun.Device, error) {
	connectionSettings, connectionSettingsErr := t.configuration.ActiveSettings()
	if connectionSettingsErr != nil {
		return nil, connectionSettingsErr
	}

	// configureTUN client
	if udpConfigurationErr := t.configureTUN(connectionSettings); udpConfigurationErr != nil {
		return nil, fmt.Errorf("failed to configure client: %v", udpConfigurationErr)
	}

	// opens the TUN device
	tunFile, openTunErr := t.ioctl.CreateTunInterface(connectionSettings.InterfaceName)
	if openTunErr != nil {
		return nil, fmt.Errorf("failed to open TUN interface: %v", openTunErr)
	}

	return t.wrapper.Wrap(tunFile)
}

// configureTUN Configures client's TUN device (creates the TUN device, assigns an IP to it, etc)
func (t *PlatformTunManager) configureTUN(connSettings settings.Settings) error {
	err := t.ip.TunTapAddDevTun(connSettings.InterfaceName)
	if err != nil {
		return err
	}

	err = t.ip.LinkSetDevUp(connSettings.InterfaceName)
	if err != nil {
		return err
	}
	fmt.Printf("created TUN interface: %v\n", connSettings.InterfaceName)

	// Assign IPv4 address to the TUN interface
	cidr4, cidr4Err := interfaceCIDR(connSettings.InterfaceIP, connSettings.InterfaceSubnet)
	if cidr4Err != nil {
		return cidr4Err
	}
	err = t.ip.AddrAddDev(connSettings.InterfaceName, cidr4)
	if err != nil {
		return err
	}
	fmt.Printf("assigned IP %s to interface %s\n", cidr4, connSettings.InterfaceName)

	// Assign IPv6 address if configured
	if connSettings.IPv6IP.IsValid() && connSettings.IPv6Subnet.IsValid() {
		cidr6, cidr6Err := interfaceCIDR(connSettings.IPv6IP, connSettings.IPv6Subnet)
		if cidr6Err != nil {
			return cidr6Err
		}
		if err := t.ip.AddrAddDev(connSettings.InterfaceName, cidr6); err != nil {
			return err
		}
		fmt.Printf("assigned IPv6 %s to interface %s\n", cidr6, connSettings.InterfaceName)
	}

	serverIP, hostErr := connSettings.Host.RouteIP()
	if hostErr != nil {
		return fmt.Errorf("failed to resolve route target host: %w", hostErr)
	}

	// Get routing information
	routeInfo, err := t.ip.RouteGet(serverIP)
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
		err = t.ip.RouteAddDev(serverIP, devInterface)
	} else {
		err = t.ip.RouteAddViaDev(serverIP, devInterface, viaGateway)
	}
	if err != nil {
		return fmt.Errorf("failed to add route to server IP: %v", err)
	}
	fmt.Printf("added route to server %s via %s dev %s\n", serverIP, viaGateway, devInterface)

	// Add route for IPv6 server address (if available)
	if !connSettings.IPv6Host.IsZero() {
		serverIPv6, ipv6HostErr := connSettings.IPv6Host.RouteIP()
		if ipv6HostErr == nil {
			routeInfo6, routeErr6 := t.ip.RouteGet(serverIPv6)
			if routeErr6 == nil {
				var via6, dev6 string
				fields6 := strings.Fields(routeInfo6)
				for i, field := range fields6 {
					if field == "via" && i+1 < len(fields6) {
						via6 = fields6[i+1]
					}
					if field == "dev" && i+1 < len(fields6) {
						dev6 = fields6[i+1]
					}
				}
				if dev6 != "" {
					if via6 == "" {
						_ = t.ip.RouteAddDev(serverIPv6, dev6)
					} else {
						_ = t.ip.RouteAddViaDev(serverIPv6, dev6, via6)
					}
					fmt.Printf("added route to IPv6 server %s via %s dev %s\n", serverIPv6, via6, dev6)
				}
			}
		}
	}

	// Set the TUN interface as the default gateway
	err = t.ip.RouteAddDefaultDev(connSettings.InterfaceName)
	if err != nil {
		return err
	}
	fmt.Printf("set %s as default gateway\n", connSettings.InterfaceName)

	// Set IPv6 default route if configured
	if connSettings.IPv6IP.IsValid() {
		if err := t.ip.Route6AddDefaultDev(connSettings.InterfaceName); err != nil {
			return err
		}
		fmt.Printf("set %s as IPv6 default gateway\n", connSettings.InterfaceName)
	}

	// sets client's TUN device maximum transmission unit (MTU)
	if setMtuErr := t.ip.LinkSetDevMTU(connSettings.InterfaceName, connSettings.MTU); setMtuErr != nil {
		return fmt.Errorf(
			"failed to set %d MTU for %s: %s", connSettings.MTU, connSettings.InterfaceName, setMtuErr,
		)
	}

	// install MSS clamping to prevent PMTU blackholes when forwarding traffic
	if err := t.mss.Install(connSettings.InterfaceName); err != nil {
		return fmt.Errorf("failed to install MSS clamping for %s: %v", connSettings.InterfaceName, err)
	}

	return nil
}

func interfaceCIDR(addr netip.Addr, subnet netip.Prefix) (string, error) {
	if !addr.IsValid() {
		return "", fmt.Errorf("invalid InterfaceIP")
	}
	if !subnet.IsValid() {
		return "", fmt.Errorf("invalid InterfaceSubnet")
	}
	return netip.PrefixFrom(addr.Unmap(), subnet.Bits()).String(), nil
}

func (t *PlatformTunManager) DisposeDevices() error {
	for _, s := range []settings.Settings{
		t.configuration.UDPSettings,
		t.configuration.TCPSettings,
		t.configuration.WSSettings,
	} {
		if err := t.mss.Remove(s.InterfaceName); err != nil {
			log.Printf("failed to remove MSS clamping for %s: %v", s.InterfaceName, err)
		}
		if routeTarget, routeErr := s.Host.RouteIP(); routeErr == nil {
			_ = t.ip.RouteDel(routeTarget)
		}
		if !s.IPv6Host.IsZero() {
			if routeTarget, routeErr := s.IPv6Host.RouteIP(); routeErr == nil {
				_ = t.ip.RouteDel(routeTarget)
			}
		}
		_ = t.ip.LinkDelete(s.InterfaceName)
	}

	return nil
}
