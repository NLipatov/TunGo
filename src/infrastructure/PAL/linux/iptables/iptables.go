package iptables

import (
	"fmt"
	"tungo/infrastructure/PAL"
)

type Wrapper struct {
	commander PAL.Commander
}

func NewWrapper(commander PAL.Commander) *Wrapper {
	return &Wrapper{commander: commander}
}

func (w *Wrapper) EnableMasquerade(devName string) error {
	output, err := w.commander.CombinedOutput("iptables", "-t", "nat",
		"-A", "POSTROUTING", "-o", devName, "-j", "MASQUERADE")
	if err != nil {
		return fmt.Errorf("failed to enable NAT on %s: %v, output: %s", devName, err, output)
	}
	return nil
}

func (w *Wrapper) DisableMasquerade(devName string) error {
	output, err := w.commander.CombinedOutput("iptables", "-t", "nat",
		"-D", "POSTROUTING", "-o", devName, "-j", "MASQUERADE")
	if err != nil {
		return fmt.Errorf("failed to disable NAT on %s: %v, output: %s", devName, err, output)
	}
	return nil
}

func (w *Wrapper) AcceptForwardFromTunToDev(tunName string, devName string) error {
	output, err := w.commander.CombinedOutput("iptables", "-A", "FORWARD",
		"-i", tunName, "-o", devName, "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to set up forwarding rule for %s -> %s: %v, output: %s",
			tunName, devName, err, output)
	}

	return nil
}

func (w *Wrapper) DropForwardFromTunToDev(tunName string, devName string) error {
	output, err := w.commander.CombinedOutput("iptables", "-D", "FORWARD",
		"-i", tunName, "-o", devName, "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf(
			"failed to remove forwarding rule for %s -> %s: %v, output: %s",
			tunName, devName, err, output)
	}

	return nil
}

func (w *Wrapper) AcceptForwardFromDevToTun(tunName string, devName string) error {
	output, err := w.commander.CombinedOutput("iptables", "-A", "FORWARD",
		"-i", devName, "-o", tunName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to set up forwarding rule for %s -> %s: %v, output: %s",
			devName, tunName, err, output)
	}

	return nil
}

func (w *Wrapper) DropForwardFromDevToTun(tunName string, devName string) error {
	output, err := w.commander.CombinedOutput("iptables", "-D", "FORWARD",
		"-i", devName, "-o", tunName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to remove forwarding rule for %s -> %s: %v, output: %s",
			devName, tunName, err, output)
	}

	return nil
}

func (w *Wrapper) ConfigureMssClamping() error {
	// Configuration for IPv4, chain FORWARD
	outputForward, errForward := w.commander.CombinedOutput("iptables", "-t", "mangle",
		"-A", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu")
	if errForward != nil {
		return fmt.Errorf("failed to configure MSS clamping on FORWARD chain: %s, output: %s",
			errForward, outputForward)
	}

	// Configuration for IPv4, chain OUTPUT
	outputOutput, errOutput := w.commander.
		CombinedOutput("iptables", "-t", "mangle",
			"-A", "OUTPUT", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu")
	if errOutput != nil {
		return fmt.Errorf("failed to configure MSS clamping on OUTPUT chain: %s, output: %s",
			errOutput, outputOutput)
	}

	// Configuration for IPv6, chain FORWARD
	outputForward6, errForward6 := w.commander.
		CombinedOutput("ip6tables", "-t", "mangle",
			"-A", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu")
	if errForward6 != nil {
		return fmt.Errorf("failed to configure IPv6 MSS clamping on FORWARD chain: %s, output: %s",
			errForward6, outputForward6)
	}

	// Configuration for IPv6, chain OUTPUT
	outputOutput6, errOutput6 := w.commander.CombinedOutput("ip6tables", "-t", "mangle", "-A",
		"OUTPUT", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu")
	if errOutput6 != nil {
		return fmt.Errorf("failed to configure IPv6 MSS clamping on OUTPUT chain: %s, output: %s",
			errOutput6, outputOutput6)
	}

	return nil
}
