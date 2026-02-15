package ip

import (
	"fmt"
	"strings"
	"tungo/infrastructure/PAL/exec_commander"
)

// Wrapper is a wrapper around ip command from the iproute2 tool collection
type Wrapper struct {
	commander exec_commander.Commander
}

func NewWrapper(commander exec_commander.Commander) Contract {
	return &Wrapper{commander: commander}
}

// TunTapAddDevTun Adds new TUN device
func (i *Wrapper) TunTapAddDevTun(devName string) error {
	createTunOutput, err := i.commander.CombinedOutput("ip", "tuntap",
		"add", "dev", devName, "mode", "tun")
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
func (i *Wrapper) AddrAddDev(devName string, cidr string) error {
	output, assignIPErr := i.commander.CombinedOutput("ip", "addr", "add", cidr, "dev", devName)
	if assignIPErr != nil {
		return fmt.Errorf("failed to assign IP to TUN %v: %v, output: %s", devName, assignIPErr, output)
	}

	return nil
}

// RouteDefault gets the default network device name.
// It checks the IPv4 routing table first, then falls back to IPv6.
func (i *Wrapper) RouteDefault() (string, error) {
	if iface, err := i.parseDefaultRoute("ip", "route"); err == nil {
		return iface, nil
	}
	if iface, err := i.parseDefaultRoute("ip", "-6", "route"); err == nil {
		return iface, nil
	}
	return "", fmt.Errorf("failed to get default interface from IPv4 or IPv6 routing table")
}

// parseDefaultRoute runs the given command and extracts the interface name
// from the first "default" route line by searching for the "dev" keyword.
func (i *Wrapper) parseDefaultRoute(name string, args ...string) (string, error) {
	out, err := i.commander.Output(name, args...)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "default") {
			fields := strings.Fields(line)
			for j, f := range fields {
				if f == "dev" && j+1 < len(fields) {
					return fields[j+1], nil
				}
			}
		}
	}
	return "", fmt.Errorf("no default route found")
}

// RouteAddDefaultDev Sets a default network device
func (i *Wrapper) RouteAddDefaultDev(devName string) error {
	output, setAsDefaultGatewayErr := i.commander.CombinedOutput("ip", "route",
		"add", "default", "dev", devName)
	if setAsDefaultGatewayErr != nil {
		return fmt.Errorf("failed to set TUN as default gateway %v: %v, output: %s",
			devName, setAsDefaultGatewayErr, output)
	}

	return nil
}

// Route6AddDefaultDev sets a default IPv6 route through the given device
func (i *Wrapper) Route6AddDefaultDev(devName string) error {
	output, err := i.commander.CombinedOutput("ip", "-6", "route",
		"add", "default", "dev", devName)
	if err != nil {
		return fmt.Errorf("failed to set IPv6 default gateway %v: %v, output: %s",
			devName, err, output)
	}
	return nil
}

// RouteAddSplitDefaultDev adds IPv4 split default routes (0.0.0.0/1 + 128.0.0.0/1)
// through the given device. These are more specific than 0.0.0.0/0 so they take
// priority without replacing the original default route. When the TUN device is
// deleted, the kernel removes these routes automatically.
func (i *Wrapper) RouteAddSplitDefaultDev(devName string) error {
	for _, prefix := range []string{"0.0.0.0/1", "128.0.0.0/1"} {
		output, err := i.commander.CombinedOutput("ip", "route", "add", prefix, "dev", devName)
		if err != nil {
			return fmt.Errorf("failed to add split route %s via %s: %v, output: %s",
				prefix, devName, err, output)
		}
	}
	return nil
}

// Route6AddSplitDefaultDev adds IPv6 split default routes (::/1 + 8000::/1)
// through the given device.
func (i *Wrapper) Route6AddSplitDefaultDev(devName string) error {
	for _, prefix := range []string{"::/1", "8000::/1"} {
		output, err := i.commander.CombinedOutput("ip", "-6", "route", "add", prefix, "dev", devName)
		if err != nil {
			return fmt.Errorf("failed to add IPv6 split route %s via %s: %v, output: %s",
				prefix, devName, err, output)
		}
	}
	return nil
}

// RouteDelSplitDefault removes IPv4 split default routes through the given device.
func (i *Wrapper) RouteDelSplitDefault(devName string) error {
	for _, prefix := range []string{"0.0.0.0/1", "128.0.0.0/1"} {
		_, _ = i.commander.CombinedOutput("ip", "route", "del", prefix, "dev", devName)
	}
	return nil
}

// Route6DelSplitDefault removes IPv6 split default routes through the given device.
func (i *Wrapper) Route6DelSplitDefault(devName string) error {
	for _, prefix := range []string{"::/1", "8000::/1"} {
		_, _ = i.commander.CombinedOutput("ip", "-6", "route", "del", prefix, "dev", devName)
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
	output, err := i.commander.CombinedOutput("ip", "link",
		"set", "dev", devName, "mtu", fmt.Sprintf("%d", mtu))
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
