package ip

// Contains wrapper-functions on ip-command

import (
	"fmt"
	"os/exec"
	"strings"
)

// LinkAdd Adds new TUN device
func LinkAdd(devName string) (string, error) {
	createTun := exec.Command("ip", "tuntap", "add", "dev", devName, "mode", "tun")
	createTunOutput, err := createTun.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create TUN %v: %v, output: %s", devName, err, createTunOutput)
	}

	return devName, nil
}

// LinkDel Deletes network device by name
func LinkDel(devName string) (string, error) {
	cmd := exec.Command("ip", "link", "delete", devName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to delete interface: %v, output: %s", err, output)
	}

	return devName, nil
}

// LinkSetUp Sets network device status as UP
func LinkSetUp(devName string) (string, error) {
	startTun := exec.Command("ip", "link", "set", "dev", devName, "up")
	startTunOutput, err := startTun.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to start TUN %v: %v, output: %s", devName, err, startTunOutput)
	}

	return devName, nil
}

// LinkAddrAdd Assigns an IP to a network device
func LinkAddrAdd(devName string, ip string) (string, error) {
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

// RouteAdd adds a route to host via device
func RouteAdd(hostIp string, ifName string) error {
	cmd := exec.Command("ip", "route", "add", hostIp, "dev", ifName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add route: %s, output: %s", err, output)
	}
	return err
}

// RouteAddViaGateway adds a route to host via device via gateway
func RouteAddViaGateway(hostIp string, ifName string, gateway string) error {
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

// SetMtu sets device mtu
func SetMtu(devName string, mtu int) error {
	cmd := exec.Command("ip", "link", "set", "dev", devName, "mtu", fmt.Sprintf("%d", mtu))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to del route: %s, output: %s", err, output)
	}
	return err
}
