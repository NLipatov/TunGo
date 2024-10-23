package ipconfiguration

import (
	"etha-tunnel/network"
	"etha-tunnel/network/ip"
	"etha-tunnel/settings/client"
	"fmt"
	"log"
	"net"
	"strings"
)

func Configure() error {
	conf, err := (&client.Conf{}).Read()
	if err != nil {
		log.Fatalf("failed to read configuration: %v", err)
	}

	// Delete existing link if any
	_, _ = ip.LinkDel(conf.TCPSettings.InterfaceName)

	name, err := network.UpNewTun(conf.TCPSettings.InterfaceName)
	if err != nil {
		return fmt.Errorf("failed to create interface %v: %v", conf.TCPSettings.InterfaceName, err)
	}
	fmt.Printf("created TUN interface: %v\n", name)

	// Assign IP address to the TUN interface
	_, err = ip.LinkAddrAdd(conf.TCPSettings.InterfaceName, conf.TCPSettings.InterfaceAddress)
	if err != nil {
		return err
	}
	fmt.Printf("assigned IP %s to interface %s\n", conf.TCPSettings.InterfaceAddress, conf.TCPSettings.InterfaceName)

	// Parse server IP
	serverIP, _, err := net.SplitHostPort(conf.ServerTCPAddress)
	if err != nil {
		return fmt.Errorf("failed to parse server address: %v", err)
	}

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
	_, err = ip.RouteAddDefaultDev(conf.TCPSettings.InterfaceName)
	if err != nil {
		return err
	}
	fmt.Printf("set %s as default gateway\n", conf.TCPSettings.InterfaceName)

	return nil
}

func Unconfigure() {
	conf, err := (&client.Conf{}).Read()
	if err != nil {
		log.Fatalf("failed to read configuration: %v", err)
	}

	hostIp, devName := strings.Split(conf.ServerTCPAddress, ":")[0], conf.TCPSettings.InterfaceName
	// Delete the route to the host IP
	if err := ip.RouteDel(hostIp); err != nil {
		log.Printf("failed to delete route: %s", err)
	}

	// Delete the TUN interface
	if _, err := ip.LinkDel(devName); err != nil {
		log.Printf("failed to delete interface: %s", err)
	}
}
