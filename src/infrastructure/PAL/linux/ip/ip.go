package ip

// Contains wrapper-functions on ip-command

import (
	"fmt"
	"os/exec"
	"strings"
)

// TunTapAddDevTun Adds new TUN device
func TunTapAddDevTun(devName string) (string, error) {
	createTun := exec.Command("ip", "tuntap", "add", "dev", devName, "mode", "tun")
	createTunOutput, err := createTun.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create TUN %v: %v, output: %s", devName, err, createTunOutput)
	}

	return devName, nil
}

// LinkDelete Deletes network device by name
func LinkDelete(devName string) (string, error) {
	cmd := exec.Command("ip", "link", "delete", devName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to delete interface: %v, output: %s", err, output)
	}

	return devName, nil
}

// LinkSetDevUp Sets network device status as UP
func LinkSetDevUp(devName string) (string, error) {
	startTun := exec.Command("ip", "link", "set", "dev", devName, "up")
	startTunOutput, err := startTun.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to start TUN %v: %v, output: %s", devName, err, startTunOutput)
	}

	return devName, nil
}

// AddrAddDev Assigns an IP to a network device
func AddrAddDev(devName string, ip string) (string, error) {
	assignIP := exec.Command("ip", "addr", "add", ip, "dev", devName)
	output, assignIPErr := assignIP.CombinedOutput()
	if assignIPErr != nil {
		return "", fmt.Errorf("failed to assign IP to TUN %v: %v, output: %s", devName, assignIPErr, output)
	}

	return devName, nil
}

// RouteDefault Gets a default network device name
func RouteDefault() (string, error) {
	out, err := exec.Command("ip", "route").Output()
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
func RouteAddDefaultDev(devName string) (string, error) {
	setAsDefaultGateway := exec.Command("ip", "route", "add", "default", "dev", devName)
	output, setAsDefaultGatewayErr := setAsDefaultGateway.CombinedOutput()
	if setAsDefaultGatewayErr != nil {
		return "", fmt.Errorf("failed to set TUN as default gateway %v: %v, output: %s", devName, setAsDefaultGatewayErr, output)
	}

	return devName, nil
}

// RouteGet gets route to host by host ip
func RouteGet(hostIp string) (string, error) {
	cmd := exec.Command("ip", "route", "get", hostIp)
	routeBytes, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get route to server IP: %v", err)
	}

	return string(routeBytes), nil
}

// RouteAddDev adds a route to host via device
func RouteAddDev(hostIp string, ifName string) error {
	cmd := exec.Command("ip", "route", "add", hostIp, "dev", ifName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add route: %s, output: %s", err, output)
	}
	return err
}

// RouteAddViaDev adds a route to host via device via gateway
func RouteAddViaDev(hostIp string, ifName string, gateway string) error {
	cmd := exec.Command("ip", "route", "add", hostIp, "via", gateway, "dev", ifName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add route: %s, output: %s", err, output)
	}
	return err
}

// RouteDel deletes a route to host
func RouteDel(hostIp string) error {
	cmd := exec.Command("ip", "route", "del", hostIp)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to del route: %s, output: %s", err, output)
	}
	return err
}

// LinkSetDevMTU sets device MTU
func LinkSetDevMTU(devName string, mtu int) error {
	cmd := exec.Command("ip", "link", "set", "dev", devName, "mtu", fmt.Sprintf("%d", mtu))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to del route: %s, output: %s", err, output)
	}
	return err
}

// AddrShowDev resolves an IP address (IPv4 or IPv6) assigned to interface
func AddrShowDev(ipV int, ifName string) (string, error) {
	cmd := exec.Command("sh", "-c", fmt.Sprintf(
		`ip -%v -o addr show dev %v | awk '{print $4}' | cut -d'/' -f1`, ipV, ifName))

	output, err := cmd.CombinedOutput()
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
