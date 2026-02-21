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
	routeEndpoint netip.AddrPort
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
	tunFile, openTunErr := t.ioctl.CreateTunInterface(connectionSettings.TunName)
	if openTunErr != nil {
		return nil, fmt.Errorf("failed to open TUN interface: %v", openTunErr)
	}

	return t.wrapper.Wrap(tunFile)
}

func (t *PlatformTunManager) SetRouteEndpoint(addr netip.AddrPort) {
	t.routeEndpoint = addr
}

// configureTUN Configures client's TUN device (creates the TUN device, assigns an IP to it, etc)
func (t *PlatformTunManager) configureTUN(connSettings settings.Settings) error {
	err := t.ip.TunTapAddDevTun(connSettings.TunName)
	if err != nil {
		return err
	}

	err = t.ip.LinkSetDevUp(connSettings.TunName)
	if err != nil {
		return err
	}
	log.Printf("created TUN interface: %v", connSettings.TunName)

	// Assign IPv4 address to the TUN interface
	cidr4, cidr4Err := connSettings.IPv4CIDR()
	if cidr4Err != nil {
		return cidr4Err
	}
	err = t.ip.AddrAddDev(connSettings.TunName, cidr4)
	if err != nil {
		return err
	}
	log.Printf("assigned IP %s to interface %s", cidr4, connSettings.TunName)

	// Assign IPv6 address if configured
	if connSettings.IPv6.IsValid() && connSettings.IPv6Subnet.IsValid() {
		cidr6, cidr6Err := connSettings.IPv6CIDR()
		if cidr6Err != nil {
			return cidr6Err
		}
		if err := t.ip.AddrAddDev(connSettings.TunName, cidr6); err != nil {
			return err
		}
		log.Printf("assigned IPv6 %s to interface %s", cidr6, connSettings.TunName)
	}

	serverIP := ""
	if t.routeEndpoint.IsValid() {
		ip := t.routeEndpoint.Addr()
		if ip.Unmap().Is4() {
			serverIP = ip.Unmap().String()
		} else {
			serverIP = ip.String()
		}
	}
	if serverIP == "" {
		var hostErr error
		serverIP, hostErr = connSettings.Server.RouteIP()
		if hostErr != nil {
			return fmt.Errorf("failed to resolve route target host: %w", hostErr)
		}
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
	log.Printf("added route to server %s via %s dev %s", serverIP, viaGateway, devInterface)

	// Add route for IPv6 server address (if available)
	if connSettings.Server.HasIPv6() || (t.routeEndpoint.IsValid() && !t.routeEndpoint.Addr().Unmap().Is4()) {
		serverIPv6 := ""
		if t.routeEndpoint.IsValid() && !t.routeEndpoint.Addr().Unmap().Is4() {
			serverIPv6 = t.routeEndpoint.Addr().String()
		}
		if serverIPv6 == "" {
			serverIPv6, _ = connSettings.Server.RouteIPv6()
		}
		if serverIPv6 != "" {
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
					log.Printf("added route to IPv6 server %s via %s dev %s", serverIPv6, via6, dev6)
				}
			}
		}
	}

	// Set split default routes — more specific than 0.0.0.0/0 so they take
	// priority without destroying the original default route. On crash or
	// device deletion the kernel removes them automatically.
	err = t.ip.RouteAddSplitDefaultDev(connSettings.TunName)
	if err != nil {
		return err
	}
	log.Printf("set %s as default gateway (split routes)", connSettings.TunName)

	// Set IPv6 split default routes if configured
	if connSettings.IPv6.IsValid() {
		if err := t.ip.Route6AddSplitDefaultDev(connSettings.TunName); err != nil {
			return err
		}
		log.Printf("set %s as IPv6 default gateway (split routes)", connSettings.TunName)
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

func (t *PlatformTunManager) DisposeDevices() error {
	for _, s := range []settings.Settings{
		t.configuration.UDPSettings,
		t.configuration.TCPSettings,
		t.configuration.WSSettings,
	} {
		if err := t.mss.Remove(s.TunName); err != nil {
			log.Printf("failed to remove MSS clamping for %s: %v", s.TunName, err)
		}
		// Remove split routes before deleting the device
		_ = t.ip.RouteDelSplitDefault(s.TunName)
		_ = t.ip.Route6DelSplitDefault(s.TunName)
		if routeTarget, routeErr := s.Server.RouteIP(); routeErr == nil {
			_ = t.ip.RouteDel(routeTarget)
		}
		if s.Server.HasIPv6() {
			if routeTarget, routeErr := s.Server.RouteIPv6(); routeErr == nil {
				_ = t.ip.RouteDel(routeTarget)
			}
		}
		_ = t.ip.LinkDelete(s.TunName)
	}

	return nil
}
