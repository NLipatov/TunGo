package tunconf

import (
	"fmt"
	"log"
	"os"
	"strings"
	"tungo/network/ip"
	"tungo/network/iptables"
	"tungo/settings"
)

// Configure configures a client TUN device
func Configure(s settings.ConnectionSettings) *os.File {
	Deconfigure(s)

	// configureTUN client
	if udpConfigurationErr := configureTUN(s); udpConfigurationErr != nil {
		log.Fatalf("failed to configure client: %v", udpConfigurationErr)
	}

	// sets client's TUN device maximum transmission unit (MTU)
	if setMtuErr := ip.LinkSetDevMtu(s.InterfaceName, s.MTU); setMtuErr != nil {
		log.Fatalf("failed to set %d MTU for %s: %s", s.MTU, s.InterfaceName, setMtuErr)
	}

	// opens the TUN device
	tunFile, openTunErr := ip.OpenTunByName(s.InterfaceName)
	if openTunErr != nil {
		log.Fatalf("failed to open TUN interface: %v", openTunErr)
	}

	return tunFile
}

// configureTUN Configures client's TUN device (creates the TUN device, assigns an IP to it, etc)
func configureTUN(connSettings settings.ConnectionSettings) error {
	// Delete existing link if any
	_, _ = ip.LinkDelete(connSettings.InterfaceName)

	name, err := ip.UpNewTun(connSettings.InterfaceName)
	if err != nil {
		return fmt.Errorf("failed to create interface %v: %v", connSettings.InterfaceName, err)
	}
	fmt.Printf("created TUN interface: %v\n", name)

	// Assign IP address to the TUN interface
	_, err = ip.AddrAddDev(connSettings.InterfaceName, connSettings.InterfaceAddress)
	if err != nil {
		return err
	}
	fmt.Printf("assigned IP %s to interface %s\n", connSettings.InterfaceAddress, connSettings.InterfaceName)

	// Parse server IP
	serverIP := connSettings.ConnectionIP

	// Get routing information
	routeInfo, err := ip.RouteGet(serverIP)
	if err != nil {
		return fmt.Errorf("failed to get route to server IP: %v", err)
	}

	var gateway, devInterface string
	fields := strings.Fields(routeInfo)
	for i, field := range fields {
		if field == "via" && i+1 < len(fields) {
			gateway = fields[i+1]
		}
		if field == "dev" && i+1 < len(fields) {
			devInterface = fields[i+1]
		}
	}

	if devInterface == "" {
		return fmt.Errorf("failed to parse route to server IP")
	}

	if gateway == "" {
		gateway, _ = ip.RouteShowDev(devInterface)
		log.Printf("No gateway found, using default gateway %s for interface %s\n", gateway, devInterface)
	}

	err = ip.RouteAddViaDev(serverIP, devInterface, gateway)
	if err != nil {
		return fmt.Errorf("failed to add route to %s via gateway %s on interface %s: %v", serverIP, gateway, devInterface, err)
	}
	log.Printf("Added route to %s via gateway %s on interface %s\n", serverIP, gateway, devInterface)

	// Check and update the TUN interface as the default gateway
	existingRoute, err := ip.RouteGet("0.0.0.0")
	if err == nil && strings.Contains(existingRoute, connSettings.InterfaceName) {
		log.Printf("Default route already exists for %s, skipping addition\n", connSettings.InterfaceName)
	} else {
		err = ip.RouteReplaceDefaultDev(connSettings.InterfaceName)
		if err != nil {
			return fmt.Errorf("failed to set default gateway to %s: %v", connSettings.InterfaceName, err)
		}
		fmt.Printf("set %s as default gateway\n", connSettings.InterfaceName)
	}

	// Configure MSS Clamping
	configureClampingErr := iptables.ConfigureMssClamping()
	if configureClampingErr != nil {
		return configureClampingErr
	}

	return nil
}

// Deconfigure does the de-configuration client device by deleting route to sever and TUN-device
func Deconfigure(connectionSettings settings.ConnectionSettings) {
	hostIp, devName := connectionSettings.ConnectionIP, connectionSettings.InterfaceName

	// Delete the route to the host IP
	if err := ip.RouteDel(hostIp); err != nil {
		log.Printf("failed to delete route: %s", err)
	}

	// Delete the TUN interface
	if err := ip.RouteDelDefaultDev(devName); err != nil {
		log.Printf("failed to delete default route for %s: %v", devName, err)
	}
	if _, err := ip.LinkDelete(devName); err != nil {
		log.Printf("failed to delete interface: %s", err)
	}

	// Restore the default route via the original gateway
	defaultGateway, defaultInterface, err := ip.RouteShowDefault()
	if err != nil {
		log.Printf("failed to restore original default route: %v", err)
		return
	}

	err = ip.RouteReplaceDefaultViaDev(defaultGateway, defaultInterface)
	if err != nil {
		log.Printf("failed to restore default route via %s dev %s: %v", defaultGateway, defaultInterface, err)
	} else {
		fmt.Printf("restored default route via %s dev %s\n", defaultGateway, defaultInterface)
	}
}
