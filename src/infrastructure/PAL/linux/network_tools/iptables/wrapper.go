package iptables

import (
	"fmt"
	"tungo/infrastructure/PAL/exec_commander"
)

type Wrapper struct {
	commander exec_commander.Commander
}

func NewWrapper(commander exec_commander.Commander) *Wrapper {
	return &Wrapper{commander: commander}
}

func (w *Wrapper) EnableDevMasquerade(devName, sourceCIDR string) error {
	args := []string{"-t", "nat", "-A", "POSTROUTING"}
	if sourceCIDR != "" {
		args = append(args, "-s", sourceCIDR)
	}
	args = append(args, "-o", devName, "-j", "MASQUERADE")
	output, err := w.commander.CombinedOutput("iptables", args...)
	if err != nil {
		return fmt.Errorf("failed to enable NAT on %s: %v, output: %s", devName, err, output)
	}
	return nil
}

func (w *Wrapper) DisableDevMasquerade(devName, sourceCIDR string) error {
	args := []string{"-t", "nat", "-D", "POSTROUTING"}
	if sourceCIDR != "" {
		args = append(args, "-s", sourceCIDR)
	}
	args = append(args, "-o", devName, "-j", "MASQUERADE")
	output, err := w.commander.CombinedOutput("iptables", args...)
	if err != nil {
		return fmt.Errorf("failed to disable NAT on %s: %v, output: %s", devName, err, output)
	}
	return nil
}

func (w *Wrapper) EnableForwardingFromTunToDev(tunName string, devName string) error {
	output, err := w.commander.CombinedOutput("iptables", "-A", "FORWARD",
		"-i", tunName, "-o", devName, "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to set up forwarding rule for %s -> %s: %v, output: %s",
			tunName, devName, err, output)
	}

	return nil
}

func (w *Wrapper) DisableForwardingFromTunToDev(tunName string, devName string) error {
	output, err := w.commander.CombinedOutput("iptables", "-D", "FORWARD",
		"-i", tunName, "-o", devName, "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf(
			"failed to remove forwarding rule for %s -> %s: %v, output: %s",
			tunName, devName, err, output)
	}

	return nil
}

func (w *Wrapper) EnableForwardingFromDevToTun(tunName string, devName string) error {
	output, err := w.commander.CombinedOutput("iptables", "-A", "FORWARD",
		"-i", devName, "-o", tunName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to set up forwarding rule for %s -> %s: %v, output: %s",
			devName, tunName, err, output)
	}

	return nil
}

func (w *Wrapper) DisableForwardingFromDevToTun(tunName string, devName string) error {
	output, err := w.commander.CombinedOutput("iptables", "-D", "FORWARD",
		"-i", devName, "-o", tunName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to remove forwarding rule for %s -> %s: %v, output: %s",
			devName, tunName, err, output)
	}

	return nil
}

func (w *Wrapper) EnableForwardingTunToTun(tunName string) error {
	output, err := w.commander.CombinedOutput("iptables", "-A", "FORWARD",
		"-i", tunName, "-o", tunName, "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to set up client-to-client forwarding rule for %s: %v, output: %s",
			tunName, err, output)
	}

	return nil
}

func (w *Wrapper) DisableForwardingTunToTun(tunName string) error {
	output, err := w.commander.CombinedOutput("iptables", "-D", "FORWARD",
		"-i", tunName, "-o", tunName, "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to remove client-to-client forwarding rule for %s: %v, output: %s",
			tunName, err, output)
	}

	return nil
}

// IPv6 (ip6tables) counterparts

func (w *Wrapper) Enable6DevMasquerade(devName, sourceCIDR string) error {
	args := []string{"-t", "nat", "-A", "POSTROUTING"}
	if sourceCIDR != "" {
		args = append(args, "-s", sourceCIDR)
	}
	args = append(args, "-o", devName, "-j", "MASQUERADE")
	output, err := w.commander.CombinedOutput("ip6tables", args...)
	if err != nil {
		return fmt.Errorf("failed to enable IPv6 NAT on %s: %v, output: %s", devName, err, output)
	}
	return nil
}

func (w *Wrapper) Disable6DevMasquerade(devName, sourceCIDR string) error {
	args := []string{"-t", "nat", "-D", "POSTROUTING"}
	if sourceCIDR != "" {
		args = append(args, "-s", sourceCIDR)
	}
	args = append(args, "-o", devName, "-j", "MASQUERADE")
	output, err := w.commander.CombinedOutput("ip6tables", args...)
	if err != nil {
		return fmt.Errorf("failed to disable IPv6 NAT on %s: %v, output: %s", devName, err, output)
	}
	return nil
}

func (w *Wrapper) Enable6ForwardingFromTunToDev(tunName string, devName string) error {
	output, err := w.commander.CombinedOutput("ip6tables", "-A", "FORWARD",
		"-i", tunName, "-o", devName, "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to set up IPv6 forwarding rule for %s -> %s: %v, output: %s",
			tunName, devName, err, output)
	}
	return nil
}

func (w *Wrapper) Disable6ForwardingFromTunToDev(tunName string, devName string) error {
	output, err := w.commander.CombinedOutput("ip6tables", "-D", "FORWARD",
		"-i", tunName, "-o", devName, "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to remove IPv6 forwarding rule for %s -> %s: %v, output: %s",
			tunName, devName, err, output)
	}
	return nil
}

func (w *Wrapper) Enable6ForwardingFromDevToTun(tunName string, devName string) error {
	output, err := w.commander.CombinedOutput("ip6tables", "-A", "FORWARD",
		"-i", devName, "-o", tunName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to set up IPv6 forwarding rule for %s -> %s: %v, output: %s",
			devName, tunName, err, output)
	}
	return nil
}

func (w *Wrapper) Disable6ForwardingFromDevToTun(tunName string, devName string) error {
	output, err := w.commander.CombinedOutput("ip6tables", "-D", "FORWARD",
		"-i", devName, "-o", tunName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to remove IPv6 forwarding rule for %s -> %s: %v, output: %s",
			devName, tunName, err, output)
	}
	return nil
}

func (w *Wrapper) Enable6ForwardingTunToTun(tunName string) error {
	output, err := w.commander.CombinedOutput("ip6tables", "-A", "FORWARD",
		"-i", tunName, "-o", tunName, "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to set up IPv6 client-to-client forwarding rule for %s: %v, output: %s",
			tunName, err, output)
	}
	return nil
}

func (w *Wrapper) Disable6ForwardingTunToTun(tunName string) error {
	output, err := w.commander.CombinedOutput("ip6tables", "-D", "FORWARD",
		"-i", tunName, "-o", tunName, "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to remove IPv6 client-to-client forwarding rule for %s: %v, output: %s",
			tunName, err, output)
	}
	return nil
}
