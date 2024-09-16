package utils

// Contains wrapper-functions on ip-command

import (
	"fmt"
	"os/exec"
	"strings"
)

// AddTun Adds new TUN device
func AddTun(name string) (string, error) {
	createTun := exec.Command("ip", "tuntap", "add", "dev", name, "mode", "tun")
	createTunOutput, err := createTun.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create TUN %v: %v, output: %s", name, err, createTunOutput)
	}

	return name, nil
}

// DelTun Deletes network device by name
func DelTun(name string) (string, error) {
	cmd := exec.Command("ip", "link", "delete", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to delete interface: %v, output: %s", err, output)
	}

	return name, nil
}

// SetTunUp Sets network device status as UP
func SetTunUp(name string) (string, error) {
	startTun := exec.Command("ip", "link", "set", "dev", name, "up")
	startTunOutput, err := startTun.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to start TUN %v: %v, output: %s", name, err, startTunOutput)
	}

	return name, nil
}

// AssignTunIP Assigns an IP to a network device
func AssignTunIP(name string, ip string) (string, error) {
	assignIP := exec.Command("ip", "addr", "add", ip, "dev", name)
	output, assignIPErr := assignIP.CombinedOutput()
	if assignIPErr != nil {
		return "", fmt.Errorf("failed to assign IP to TUN %v: %v, output: %s", name, assignIPErr, output)
	}

	return name, nil
}

// GetDefaultIf Gets a default network device name
func GetDefaultIf() (string, error) {
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

// SetDefaultIf Sets a default network device
func SetDefaultIf(name string) (string, error) {
	setAsDefaultGateway := exec.Command("ip", "route", "add", "default", "dev", name)
	output, setAsDefaultGatewayErr := setAsDefaultGateway.CombinedOutput()
	if setAsDefaultGatewayErr != nil {
		return "", fmt.Errorf("failed to set TUN as default gateway %v: %v, output: %s", name, setAsDefaultGatewayErr, output)
	}

	return name, nil
}
