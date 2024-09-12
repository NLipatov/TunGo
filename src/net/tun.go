package net

import (
	"fmt"
	"os/exec"
)

func CreateTun(ifName string) (string, error) {
	cmd := exec.Command("ip", "tuntap", "add", "dev", ifName, "mode", "tun")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create TUN %v: %v, output: %s", ifName, err, output)
	}

	return ifName, nil
}

func DeleteInterface(ifName string) error {
	cmd := exec.Command("ip", "link", "delete", ifName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete interface: %v, output: %s", err, output)
	}
	return nil
}
