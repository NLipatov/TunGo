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

	// Assign IP address to the TUN interface
	interfaceCIDR, interfaceCIDRErr := interfaceCIDR(connSettings.InterfaceIP, connSettings.InterfaceSubnet)
	if interfaceCIDRErr != nil {
		return interfaceCIDRErr
	}
	err = t.ip.AddrAddDev(connSettings.InterfaceName, interfaceCIDR)
	if err != nil {
		return err
	}
	fmt.Printf("assigned IP %s to interface %s\n", interfaceCIDR, connSettings.InterfaceName)

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

	// Set the TUN interface as the default gateway
	err = t.ip.RouteAddDefaultDev(connSettings.InterfaceName)
	if err != nil {
		return err
	}
	fmt.Printf("set %s as default gateway\n", connSettings.InterfaceName)

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
	if err := t.mss.Remove(t.configuration.UDPSettings.InterfaceName); err != nil {
		log.Printf("failed to remove MSS clamping for %s: %v", t.configuration.UDPSettings.InterfaceName, err)
	}
	if routeTarget, routeErr := t.configuration.UDPSettings.Host.RouteIP(); routeErr == nil {
		_ = t.ip.RouteDel(routeTarget)
	}
	_ = t.ip.LinkDelete(t.configuration.UDPSettings.InterfaceName)

	if err := t.mss.Remove(t.configuration.TCPSettings.InterfaceName); err != nil {
		log.Printf("failed to remove MSS clamping for %s: %v", t.configuration.TCPSettings.InterfaceName, err)
	}
	if routeTarget, routeErr := t.configuration.TCPSettings.Host.RouteIP(); routeErr == nil {
		_ = t.ip.RouteDel(routeTarget)
	}
	_ = t.ip.LinkDelete(t.configuration.TCPSettings.InterfaceName)

	if err := t.mss.Remove(t.configuration.WSSettings.InterfaceName); err != nil {
		log.Printf("failed to remove MSS clamping for %s: %v", t.configuration.WSSettings.InterfaceName, err)
	}
	if routeTarget, routeErr := t.configuration.WSSettings.Host.RouteIP(); routeErr == nil {
		_ = t.ip.RouteDel(routeTarget)
	}
	_ = t.ip.LinkDelete(t.configuration.WSSettings.InterfaceName)

	return nil
}
