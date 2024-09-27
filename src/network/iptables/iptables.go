package iptables

import (
	"fmt"
	"os/exec"
)

func EnableMasquerade(devName string) error {
	cmd := exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-o", devName, "-j", "MASQUERADE")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to enable NAT on %s: %v, output: %s", devName, err, output)
	}
	return nil
}

func DisableMasquerade(devName string) error {
	cmd := exec.Command("iptables", "-t", "nat", "-D", "POSTROUTING", "-o", devName, "-j", "MASQUERADE")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to disable NAT on %s: %v, output: %s", devName, err, output)
	}
	return nil
}

func AcceptForwardFromTunToDev(tunName string, devName string) error {
	cmd := exec.Command("iptables", "-A", "FORWARD", "-i", tunName, "-o", devName, "-j", "ACCEPT")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set up forwarding rule for %s -> %s: %v, output: %s", tunName, devName, err, output)
	}

	return nil
}

func DropForwardFromTunToDev(tunName string, devName string) error {
	cmd := exec.Command("iptables", "-D", "FORWARD", "-i", tunName, "-o", devName, "-j", "ACCEPT")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove forwarding rule for %s -> %s: %v, output: %s", tunName, devName, err, output)
	}

	return nil
}

func AcceptForwardFromDevToTun(tunName string, devName string) error {
	cmd := exec.Command("iptables", "-A", "FORWARD", "-i", devName, "-o", tunName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set up forwarding rule for %s -> %s: %v, output: %s", devName, tunName, err, output)
	}

	return nil
}

func DropForwardFromDevToTun(tunName string, devName string) error {
	cmd := exec.Command("iptables", "-D", "FORWARD", "-i", devName, "-o", tunName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove forwarding rule for %s -> %s: %v, output: %s", devName, tunName, err, output)
	}

	return nil
}
