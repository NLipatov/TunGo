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

func ConfigureMssClamping() error {
	// Configuration for IPv4, chain FORWARD
	cmdForward := exec.Command("iptables", "-t", "mangle", "-A", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu")
	outputForward, errForward := cmdForward.CombinedOutput()
	if errForward != nil {
		return fmt.Errorf("failed to configure MSS clamping on FORWARD chain: %s, output: %s", errForward, outputForward)
	}

	// Configuration for IPv4, chain OUTPUT
	cmdOutput := exec.Command("iptables", "-t", "mangle", "-A", "OUTPUT", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu")
	outputOutput, errOutput := cmdOutput.CombinedOutput()
	if errOutput != nil {
		return fmt.Errorf("failed to configure MSS clamping on OUTPUT chain: %s, output: %s", errOutput, outputOutput)
	}

	// Configuration for IPv6, chain FORWARD
	cmdForward6 := exec.Command("ip6tables", "-t", "mangle", "-A", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu")
	outputForward6, errForward6 := cmdForward6.CombinedOutput()
	if errForward6 != nil {
		return fmt.Errorf("failed to configure IPv6 MSS clamping on FORWARD chain: %s, output: %s", errForward6, outputForward6)
	}

	// Configuration for IPv6, chain OUTPUT
	cmdOutput6 := exec.Command("ip6tables", "-t", "mangle", "-A", "OUTPUT", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu")
	outputOutput6, errOutput6 := cmdOutput6.CombinedOutput()
	if errOutput6 != nil {
		return fmt.Errorf("failed to configure IPv6 MSS clamping on OUTPUT chain: %s, output: %s", errOutput6, outputOutput6)
	}

	return nil
}
