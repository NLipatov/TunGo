package clientipconf

import (
	"fmt"
	"log"
	"strings"
	"tungo/network/ip"
	"tungo/network/iptables"
	"tungo/settings"
)

func Configure(connSettings settings.ConnectionSettings) error {
	// Delete existing link if any
	_, _ = ip.LinkDel(connSettings.InterfaceName)

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

func Unconfigure(connectionSettings settings.ConnectionSettings) {
	hostIp, devName := connectionSettings.ConnectionIP, connectionSettings.InterfaceName
	// Delete the route to the host IP
	if err := ip.RouteDel(hostIp); err != nil {
		log.Printf("failed to delete route: %s", err)
	}

	// Delete the TUN interface
	if _, err := ip.LinkDel(devName); err != nil {
		log.Printf("failed to delete interface: %s", err)
	}
}
