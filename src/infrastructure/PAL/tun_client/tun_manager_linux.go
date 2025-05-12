package tun_client

import (
	"fmt"
	"strings"
	"tungo/application"
	"tungo/infrastructure/PAL/linux/ip"
	"tungo/infrastructure/PAL/linux/iptables"
	"tungo/settings"
	"tungo/settings/client_configuration"
)

// PlatformTunManager Linux-specific TunDevice manager
type PlatformTunManager struct {
	conf client_configuration.Configuration
}

func NewPlatformTunManager(conf client_configuration.Configuration) (application.ClientTunManager, error) {
	return &PlatformTunManager{
		conf: conf,
	}, nil
}

func (t *PlatformTunManager) CreateTunDevice() (application.TunDevice, error) {
	var s settings.ConnectionSettings
	switch t.conf.Protocol {
	case settings.UDP:
		s = t.conf.UDPSettings
	case settings.TCP:
		s = t.conf.TCPSettings
	default:
		return nil, fmt.Errorf("unsupported protocol")
	}

	// configureTUN client
	if udpConfigurationErr := t.configureTUN(s); udpConfigurationErr != nil {
		return nil, fmt.Errorf("failed to configure client: %v", udpConfigurationErr)
	}

	// opens the TUN device
	tunFile, openTunErr := ip.OpenTunByName(s.InterfaceName)
	if openTunErr != nil {
		return nil, fmt.Errorf("failed to open TUN interface: %v", openTunErr)
	}

	return tunFile, nil
}

// configureTUN Configures client's TUN device (creates the TUN device, assigns an IP to it, etc)
func (t *PlatformTunManager) configureTUN(connSettings settings.ConnectionSettings) error {
	name, err := ip.UpNewTun(connSettings.InterfaceName)
	if err != nil {
		return fmt.Errorf("failed to create interface %v: %v", connSettings.InterfaceName, err)
	}
	fmt.Printf("created TUN interface: %v\n", name)

	// Assign IP address to the TUN interface
	_, err = ip.LinkAddrAdd(connSettings.InterfaceName, connSettings.InterfaceAddress)
	if err != nil {
		return err
	}
	fmt.Printf("assigned IP %s to interface %s\n", connSettings.InterfaceAddress, connSettings.InterfaceName)

	// Parse server IP
	serverIP := connSettings.ConnectionIP

	// Get routing information
	routeInfo, err := ip.RouteGet(serverIP)
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
		err = ip.RouteAdd(serverIP, devInterface)
	} else {
		err = ip.RouteAddViaGateway(serverIP, devInterface, viaGateway)
	}
	if err != nil {
		return fmt.Errorf("failed to add route to server IP: %v", err)
	}
	fmt.Printf("added route to server %s via %s dev %s\n", serverIP, viaGateway, devInterface)

	// Set the TUN interface as the default gateway
	_, err = ip.RouteAddDefaultDev(connSettings.InterfaceName)
	if err != nil {
		return err
	}
	fmt.Printf("set %s as default gateway\n", connSettings.InterfaceName)

	configureClampingErr := iptables.ConfigureMssClamping()
	if configureClampingErr != nil {
		return configureClampingErr
	}

	// sets client's TUN device maximum transmission unit (MTU)
	if setMtuErr := ip.SetMtu(connSettings.InterfaceName, connSettings.MTU); setMtuErr != nil {
		return fmt.Errorf("failed to set %d MTU for %s: %s", connSettings.MTU, connSettings.InterfaceName, setMtuErr)
	}

	return nil
}

func (t *PlatformTunManager) DisposeTunDevices() error {
	_ = ip.RouteDel(t.conf.UDPSettings.ConnectionIP)
	_, _ = ip.LinkDel(t.conf.UDPSettings.InterfaceName)

	_ = ip.RouteDel(t.conf.TCPSettings.ConnectionIP)
	_, _ = ip.LinkDel(t.conf.TCPSettings.InterfaceName)

	return nil
}
