package iptables

import (
	"fmt"
	"tungo/internal/platform/command"
)

type Configurator struct {
	commander command.Runner
}

func New(commander command.Runner) *Configurator {
	return &Configurator{commander: commander}
}

func (w *Configurator) EnableDevMasquerade(devName, sourceCIDR string) error {
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

func (w *Configurator) DisableDevMasquerade(devName, sourceCIDR string) error {
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

func (w *Configurator) EnableForwardingFromTunToDev(tunName string, devName string) error {
	output, err := w.commander.CombinedOutput("iptables", "-A", "FORWARD",
		"-i", tunName, "-o", devName, "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to set up forwarding rule for %s -> %s: %v, output: %s",
			tunName, devName, err, output)
	}

	return nil
}

func (w *Configurator) DisableForwardingFromTunToDev(tunName string, devName string) error {
	output, err := w.commander.CombinedOutput("iptables", "-D", "FORWARD",
		"-i", tunName, "-o", devName, "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf(
			"failed to remove forwarding rule for %s -> %s: %v, output: %s",
			tunName, devName, err, output)
	}

	return nil
}

func (w *Configurator) EnableForwardingFromDevToTun(tunName string, devName string) error {
	output, err := w.commander.CombinedOutput("iptables", "-A", "FORWARD",
		"-i", devName, "-o", tunName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to set up forwarding rule for %s -> %s: %v, output: %s",
			devName, tunName, err, output)
	}

	return nil
}

func (w *Configurator) DisableForwardingFromDevToTun(tunName string, devName string) error {
	output, err := w.commander.CombinedOutput("iptables", "-D", "FORWARD",
		"-i", devName, "-o", tunName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to remove forwarding rule for %s -> %s: %v, output: %s",
			devName, tunName, err, output)
	}

	return nil
}

func (w *Configurator) EnableForwardingTunToTun(tunName string) error {
	output, err := w.commander.CombinedOutput("iptables", "-A", "FORWARD",
		"-i", tunName, "-o", tunName, "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to set up client-to-client forwarding rule for %s: %v, output: %s",
			tunName, err, output)
	}

	return nil
}

func (w *Configurator) DisableForwardingTunToTun(tunName string) error {
	output, err := w.commander.CombinedOutput("iptables", "-D", "FORWARD",
		"-i", tunName, "-o", tunName, "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to remove client-to-client forwarding rule for %s: %v, output: %s",
			tunName, err, output)
	}

	return nil
}

// IPv6 (ip6tables) counterparts

func (w *Configurator) Enable6DevMasquerade(devName, sourceCIDR string) error {
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

func (w *Configurator) Disable6DevMasquerade(devName, sourceCIDR string) error {
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

func (w *Configurator) Enable6ForwardingFromTunToDev(tunName string, devName string) error {
	output, err := w.commander.CombinedOutput("ip6tables", "-A", "FORWARD",
		"-i", tunName, "-o", devName, "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to set up IPv6 forwarding rule for %s -> %s: %v, output: %s",
			tunName, devName, err, output)
	}
	return nil
}

func (w *Configurator) Disable6ForwardingFromTunToDev(tunName string, devName string) error {
	output, err := w.commander.CombinedOutput("ip6tables", "-D", "FORWARD",
		"-i", tunName, "-o", devName, "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to remove IPv6 forwarding rule for %s -> %s: %v, output: %s",
			tunName, devName, err, output)
	}
	return nil
}

func (w *Configurator) Enable6ForwardingFromDevToTun(tunName string, devName string) error {
	output, err := w.commander.CombinedOutput("ip6tables", "-A", "FORWARD",
		"-i", devName, "-o", tunName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to set up IPv6 forwarding rule for %s -> %s: %v, output: %s",
			devName, tunName, err, output)
	}
	return nil
}

func (w *Configurator) Disable6ForwardingFromDevToTun(tunName string, devName string) error {
	output, err := w.commander.CombinedOutput("ip6tables", "-D", "FORWARD",
		"-i", devName, "-o", tunName, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to remove IPv6 forwarding rule for %s -> %s: %v, output: %s",
			devName, tunName, err, output)
	}
	return nil
}

func (w *Configurator) Enable6ForwardingTunToTun(tunName string) error {
	output, err := w.commander.CombinedOutput("ip6tables", "-A", "FORWARD",
		"-i", tunName, "-o", tunName, "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to set up IPv6 client-to-client forwarding rule for %s: %v, output: %s",
			tunName, err, output)
	}
	return nil
}

func (w *Configurator) Disable6ForwardingTunToTun(tunName string) error {
	output, err := w.commander.CombinedOutput("ip6tables", "-D", "FORWARD",
		"-i", tunName, "-o", tunName, "-j", "ACCEPT")
	if err != nil {
		return fmt.Errorf("failed to remove IPv6 client-to-client forwarding rule for %s: %v, output: %s",
			tunName, err, output)
	}
	return nil
}
