package tun_configurator

import (
	"fmt"
	"strings"
	"tungo/application"
	ip2 "tungo/infrastructure/network/ip"
	"tungo/infrastructure/network/iptables"
	"tungo/settings"
)

// LinuxTunConfigurator platform specific TUN-configurator used for Linux platform
type LinuxTunConfigurator struct {
}

// Configure configures a client TUN device
func (t *LinuxTunConfigurator) Configure(s settings.ConnectionSettings) (application.TunDevice, error) {
	// configureTUN client
	if udpConfigurationErr := configureTUN(s); udpConfigurationErr != nil {
		return nil, fmt.Errorf("failed to configure client: %v", udpConfigurationErr)
	}

	// sets client's TUN device maximum transmission unit (MTU)
	if setMtuErr := ip2.SetMtu(s.InterfaceName, s.MTU); setMtuErr != nil {
		return nil, fmt.Errorf("failed to set %d MTU for %s: %s", s.MTU, s.InterfaceName, setMtuErr)
	}

	// opens the TUN device
	tunFile, openTunErr := ip2.OpenTunByName(s.InterfaceName)
	if openTunErr != nil {
		return nil, fmt.Errorf("failed to open TUN interface: %v", openTunErr)
	}

	return tunFile, nil
}

// configureTUN Configures client's TUN device (creates the TUN device, assigns an IP to it, etc)
func configureTUN(connSettings settings.ConnectionSettings) error {
	name, err := ip2.UpNewTun(connSettings.InterfaceName)
	if err != nil {
		return fmt.Errorf("failed to create interface %v: %v", connSettings.InterfaceName, err)
	}
	fmt.Printf("created TUN interface: %v\n", name)

	// Assign IP address to the TUN interface
	_, err = ip2.LinkAddrAdd(connSettings.InterfaceName, connSettings.InterfaceAddress)
	if err != nil {
		return err
	}
	fmt.Printf("assigned IP %s to interface %s\n", connSettings.InterfaceAddress, connSettings.InterfaceName)

	// Parse server IP
	serverIP := connSettings.ConnectionIP

	// Get routing information
	routeInfo, err := ip2.RouteGet(serverIP)
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
		err = ip2.RouteAdd(serverIP, devInterface)
	} else {
		err = ip2.RouteAddViaGateway(serverIP, devInterface, viaGateway)
	}
	if err != nil {
		return fmt.Errorf("failed to add route to server IP: %v", err)
	}
	fmt.Printf("added route to server %s via %s dev %s\n", serverIP, viaGateway, devInterface)

	// Set the TUN interface as the default gateway
	_, err = ip2.RouteAddDefaultDev(connSettings.InterfaceName)
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

// Deconfigure does the de-configuration client device by deleting route to sever and TUN-device
func (t *LinuxTunConfigurator) Deconfigure(connectionSettings settings.ConnectionSettings) {
	// Delete route to server
	_ = ip2.RouteDel(connectionSettings.ConnectionIP)
	// Delete the TUN interface
	_, _ = ip2.LinkDel(connectionSettings.InterfaceName)
}
