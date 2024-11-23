package ip

// Contains wrapper-functions on ip-command

import (
	"fmt"
	"os/exec"
	"strings"
)

// AddTunDev Adds new TUN device
func AddTunDev(devName string) (string, error) {
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

// Route Gets a default network device name
func Route() (string, error) {
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

// RouteGet gets route to host by host ip
func RouteGet(hostIp string) (string, error) {
	cmd := exec.Command("ip", "route", "get", hostIp)
	routeBytes, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get route to server IP: %v", err)
	}

	return string(routeBytes), nil
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

// LinkSetDevMtu sets device mtu
func LinkSetDevMtu(devName string, mtu int) error {
	cmd := exec.Command("ip", "link", "set", "dev", devName, "mtu", fmt.Sprintf("%d", mtu))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to del route: %s, output: %s", err, output)
	}
	return err
}

// GetDevAddr resolves an IP address (IPv4 or IPv6) assigned to interface
func GetDevAddr(ipV int, ifName string) (string, error) {
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

// RouteShowDev retrieves the default gateway for a given interface
func RouteShowDev(devName string) (string, error) {
	out, err := exec.Command("ip", "route", "show", "dev", devName).Output()
	if err != nil {
		return "", fmt.Errorf("failed to retrieve default gateway for interface %s: %v", devName, err)
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.Contains(line, "default via") {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				return fields[2], nil // Return the "via" field (gateway)
			}
		}
	}
	return "", fmt.Errorf("no default gateway found for interface %s", devName)
}

// RouteShowDefault retrieves the original default route (gateway and interface)
func RouteShowDefault() (string, string, error) {
	out, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to retrieve original default route: %v", err)
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "default") {
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				return fields[2], fields[4], nil // Gateway and Interface
			}
		}
	}
	return "", "", fmt.Errorf("no default route found")
}

func RouteReplaceDefaultDev(devName string) error {
	_, err := exec.Command("ip", "route", "replace", "default", "dev", devName).CombinedOutput()
	return err
}

func RouteDelDefaultDev(devName string) error {
	_, err := exec.Command("ip", "route", "del", "default", "dev", devName).CombinedOutput()
	return err
}

func RouteReplaceDefaultViaDev(defaultGateway, dev string) error {
	_, err := exec.Command("ip", "route", "replace", "default", "via", defaultGateway, "dev", dev).CombinedOutput()
	return err
}
