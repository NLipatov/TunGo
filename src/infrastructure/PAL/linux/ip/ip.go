package ip

import (
	"fmt"
	"strings"
	"tungo/infrastructure/PAL"
)

// Wrapper is a wrapper around ip command from the iproute2 tool collection
type Wrapper struct {
	commander PAL.Commander
}

func NewWrapper(commander PAL.Commander) Contract {
	return &Wrapper{commander: commander}
}

// TunTapAddDevTun Adds new TUN device
func (i *Wrapper) TunTapAddDevTun(devName string) error {
	createTunOutput, err := i.commander.CombinedOutput("ip", "tuntap", "add", "dev", devName, "mode", "tun")
	if err != nil {
		return fmt.Errorf("failed to create TUN %v: %v, output: %s", devName, err, createTunOutput)
	}

	return nil
}

// LinkDelete Deletes network device by name
func (i *Wrapper) LinkDelete(devName string) error {
	output, err := i.commander.CombinedOutput("ip", "link", "delete", devName)
	if err != nil {
		return fmt.Errorf("failed to delete interface: %v, output: %s", err, output)
	}

	return nil
}

// LinkSetDevUp Sets network device status as UP
func (i *Wrapper) LinkSetDevUp(devName string) error {
	startTunOutput, err := i.commander.CombinedOutput("ip", "link", "set", "dev", devName, "up")
	if err != nil {
		return fmt.Errorf("failed to start TUN %v: %v, output: %s", devName, err, startTunOutput)
	}

	return nil
}

// AddrAddDev Assigns an IP to a network device
func (i *Wrapper) AddrAddDev(devName string, ip string) error {
	output, assignIPErr := i.commander.CombinedOutput("ip", "addr", "add", ip, "dev", devName)
	if assignIPErr != nil {
		return fmt.Errorf("failed to assign IP to TUN %v: %v, output: %s", devName, assignIPErr, output)
	}

	return nil
}

// RouteDefault Gets a default network device name
func (i *Wrapper) RouteDefault() (string, error) {
	out, err := i.commander.Output("ip", "route")
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "default") {
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				return fields[4], nil
			}
		}
	}
	return "", fmt.Errorf("failed to get default interface")
}

// RouteAddDefaultDev Sets a default network device
func (i *Wrapper) RouteAddDefaultDev(devName string) error {
	output, setAsDefaultGatewayErr := i.commander.CombinedOutput("ip", "route", "add", "default", "dev", devName)
	if setAsDefaultGatewayErr != nil {
		return fmt.Errorf("failed to set TUN as default gateway %v: %v, output: %s", devName, setAsDefaultGatewayErr, output)
	}

	return nil
}

// RouteGet gets route to host by host ip
func (i *Wrapper) RouteGet(hostIp string) (string, error) {
	routeBytes, err := i.commander.Output("ip", "route", "get", hostIp)
	if err != nil {
		return "", fmt.Errorf("failed to get route to server IP: %v", err)
	}

	return string(routeBytes), nil
}

// RouteAddDev adds a route to host via device
func (i *Wrapper) RouteAddDev(hostIp string, ifName string) error {
	output, err := i.commander.CombinedOutput("ip", "route", "add", hostIp, "dev", ifName)
	if err != nil {
		return fmt.Errorf("failed to add route: %s, output: %s", err, output)
	}
	return err
}

// RouteAddViaDev adds a route to host via device via gateway
func (i *Wrapper) RouteAddViaDev(hostIp string, ifName string, gateway string) error {
	output, err := i.commander.CombinedOutput("ip", "route", "add", hostIp, "via", gateway, "dev", ifName)
	if err != nil {
		return fmt.Errorf("failed to add route: %s, output: %s", err, output)
	}
	return err
}

// RouteDel deletes a route to host
func (i *Wrapper) RouteDel(hostIp string) error {
	output, err := i.commander.CombinedOutput("ip", "route", "del", hostIp)
	if err != nil {
		return fmt.Errorf("failed to del route: %s, output: %s", err, output)
	}
	return err
}

// LinkSetDevMTU sets device MTU
func (i *Wrapper) LinkSetDevMTU(devName string, mtu int) error {
	output, err := i.commander.CombinedOutput("ip", "link", "set", "dev", devName, "mtu", fmt.Sprintf("%d", mtu))
	if err != nil {
		return fmt.Errorf("failed to set mtu: %s, output: %s", err, output)
	}
	return err
}

// AddrShowDev resolves an IP address (IPv4 or IPv6) assigned to interface
func (i *Wrapper) AddrShowDev(ipV int, ifName string) (string, error) {
	output, err := i.commander.CombinedOutput("sh", "-c", fmt.Sprintf(
		`ip -%v -o addr show dev %v | awk '{print $4}' | cut -d'/' -f1`, ipV, ifName))
	if err != nil {
		return "", fmt.Errorf(
			"failed to get IP for interface %s: %v (%s)", ifName, err, strings.TrimSpace(string(output)))
	}

	ip := strings.TrimSpace(string(output))
	if ip == "" {
		return "", fmt.Errorf("no IP address found for interface %s", ifName)
	}

	return ip, nil
}
