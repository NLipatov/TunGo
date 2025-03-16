package tun_device

import (
	"fmt"
	"strings"
	"tungo/application"
	"tungo/infrastructure/network/ip"
	"tungo/infrastructure/network/iptables"
	"tungo/settings"
	"tungo/settings/client"
)

// linuxTunDeviceManager Linux-specific TunDevice manager
type linuxTunDeviceManager struct {
	tunSettings settings.ConnectionSettings
}

func newLinuxTunDeviceManager(conf client.Conf) (application.TunDevice, error) {
	tcpDevManager := &linuxTunDeviceManager{
		tunSettings: conf.TCPSettings,
	}

	udpDevManager := &linuxTunDeviceManager{
		tunSettings: conf.UDPSettings,
	}

	tcpDevManager.DisposeTunDevice()
	udpDevManager.DisposeTunDevice()

	switch conf.Protocol {
	case settings.TCP:
		return tcpDevManager.NewTunDevice()
	case settings.UDP:
		return udpDevManager.NewTunDevice()
	default:
		return nil, fmt.Errorf("unsupported protocol")
	}
}

func (t *linuxTunDeviceManager) NewTunDevice() (application.TunDevice, error) {
	// configureTUN client
	if udpConfigurationErr := configureTUN(t.tunSettings); udpConfigurationErr != nil {
		return nil, fmt.Errorf("failed to configure client: %v", udpConfigurationErr)
	}

	// sets client's TUN device maximum transmission unit (MTU)
	if setMtuErr := ip.SetMtu(t.tunSettings.InterfaceName, t.tunSettings.MTU); setMtuErr != nil {
		return nil, fmt.Errorf("failed to set %d MTU for %s: %s", t.tunSettings.MTU, t.tunSettings.InterfaceName, setMtuErr)
	}

	// opens the TUN device
	tunFile, openTunErr := ip.OpenTunByName(t.tunSettings.InterfaceName)
	if openTunErr != nil {
		return nil, fmt.Errorf("failed to open TUN interface: %v", openTunErr)
	}

	return tunFile, nil
}

// configureTUN Configures client's TUN device (creates the TUN device, assigns an IP to it, etc)
func configureTUN(connSettings settings.ConnectionSettings) error {
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

	return nil
}

func (t *linuxTunDeviceManager) DisposeTunDevice() {
	// Delete route to server
	_ = ip.RouteDel(t.tunSettings.ConnectionIP)
	// Delete the TUN interface
	_, _ = ip.LinkDel(t.tunSettings.InterfaceName)
}
