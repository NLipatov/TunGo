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

func (w *Wrapper) EnableDevMasquerade(devName string) error {
	output, err := w.commander.CombinedOutput("iptables", "-t", "nat",
		"-A", "POSTROUTING", "-o", devName, "-j", "MASQUERADE")
	if err != nil {
		return fmt.Errorf("failed to enable NAT on %s: %v, output: %s", devName, err, output)
	}
	return nil
}

func (w *Wrapper) DisableDevMasquerade(devName string) error {
	output, err := w.commander.CombinedOutput("iptables", "-t", "nat",
		"-D", "POSTROUTING", "-o", devName, "-j", "MASQUERADE")
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
