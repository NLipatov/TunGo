package tun_client

import (
	"fmt"
	"strings"
	"tungo/application/network/tun"
	"tungo/infrastructure/PAL"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/linux/network_tools/ioctl"
	"tungo/infrastructure/PAL/linux/network_tools/ip"
	"tungo/infrastructure/settings"
)

// PlatformTunManager Linux-specific TunDevice manager
type PlatformTunManager struct {
	conf  client.Configuration
	ip    ip.Contract
	ioctl ioctl.Contract
}

func NewPlatformTunManager(
	conf client.Configuration,
) (tun.ClientManager, error) {
	return &PlatformTunManager{
		conf:  conf,
		ip:    ip.NewWrapper(PAL.NewExecCommander()),
		ioctl: ioctl.NewWrapper(ioctl.NewLinuxIoctlCommander(), "/dev/net/tun"),
	}, nil
}

func (t *PlatformTunManager) CreateDevice() (tun.Device, error) {
	var s settings.Settings
	switch t.conf.Protocol {
	case settings.UDP:
		s = t.conf.UDPSettings
	case settings.TCP:
		s = t.conf.TCPSettings
	case settings.WS, settings.WSS:
		s = t.conf.WSSettings
	default:
		return nil, fmt.Errorf("unsupported protocol")
	}

	// configureTUN client
	if udpConfigurationErr := t.configureTUN(s); udpConfigurationErr != nil {
		return nil, fmt.Errorf("failed to configure client: %v", udpConfigurationErr)
	}

	// opens the TUN device
	tunFile, openTunErr := t.ioctl.CreateTunInterface(s.InterfaceName)
	if openTunErr != nil {
		return nil, fmt.Errorf("failed to open TUN interface: %v", openTunErr)
	}

	return tunFile, nil
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
	err = t.ip.AddrAddDev(connSettings.InterfaceName, connSettings.InterfaceAddress)
	if err != nil {
		return err
	}
	fmt.Printf("assigned IP %s to interface %s\n", connSettings.InterfaceAddress, connSettings.InterfaceName)

	// Parse server IP
	serverIP := connSettings.ConnectionIP

	// Get routing information
	routeInfo, err := t.ip.RouteGet(serverIP)
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
		return fmt.Errorf("failed to set %d MTU for %s: %s", connSettings.MTU, connSettings.InterfaceName, setMtuErr)
	}

	return nil
}

func (t *PlatformTunManager) DisposeDevices() error {
	_ = t.ip.RouteDel(t.conf.UDPSettings.ConnectionIP)
	_ = t.ip.LinkDelete(t.conf.UDPSettings.InterfaceName)

	_ = t.ip.RouteDel(t.conf.TCPSettings.ConnectionIP)
	_ = t.ip.LinkDelete(t.conf.TCPSettings.InterfaceName)

	_ = t.ip.RouteDel(t.conf.WSSettings.ConnectionIP)
	_ = t.ip.LinkDelete(t.conf.WSSettings.InterfaceName)

	return nil
}
