package net

import (
	"fmt"
	"os/exec"
	"unsafe"
)

const (
	IFNAMSIZ  = 16         // Max if name size, bytes
	TUNSETIFF = 0x400454ca // Code to create TUN/TAP if via ioctl
	IFF_TUN   = 0x0001     // Enabling TUN flag
	IFF_NO_PI = 0x1000     // Disabling PI (Packet Information)
)

type ifreq struct {
	Name  [IFNAMSIZ]byte
	Flags uint16
	_     [24]byte
}

func UpNewTun(ifName string) (string, error) {
	createTun := exec.Command("ip", "tuntap", "add", "dev", ifName, "mode", "tun")
	createTunOutput, err := createTun.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create TUN %v: %v, output: %s", ifName, err, createTunOutput)
	}

	startTun := exec.Command("ip", "link", "set", "dev", ifName, "up")
	startTunOutput, err := startTun.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to start TUN %v: %v, output: %s", ifName, err, startTunOutput)
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
