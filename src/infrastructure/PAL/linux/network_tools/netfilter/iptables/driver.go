package iptables

import (
	"errors"
	"fmt"
	"tungo/infrastructure/PAL"
)

type Wrapper struct {
	commander          PAL.Commander
	v4binary, v6binary string
}

func New(
	v4binary, v6binary string,
	commander PAL.Commander,
) *Wrapper {
	return &Wrapper{
		commander: commander,
		v4binary:  v4binary,
		v6binary:  v6binary,
	}
}

func (w *Wrapper) EnableDevMasquerade(devName string) error {
	if _, err := w.execWithBinary("-t", "nat", "-A", "POSTROUTING", "-o", devName, "-j", "MASQUERADE"); err != nil {
		return fmt.Errorf("failed to enable NAT on %s: %v", devName, err)
	}
	return nil
}

func (w *Wrapper) DisableDevMasquerade(devName string) error {
	if _, err := w.execWithBinary("-t", "nat", "-D", "POSTROUTING", "-o", devName, "-j", "MASQUERADE"); err != nil {
		return fmt.Errorf("failed to disable NAT on %s: %v", devName, err)
	}
	return nil
}

func (w *Wrapper) EnableForwardingFromTunToDev(tunName string, devName string) error {
	if _, err := w.execWithBinary("-A", "FORWARD", "-i", tunName, "-o", devName, "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("failed to set up forwarding rule for %s -> %s: %v",
			tunName, devName, err)
	}

	return nil
}

func (w *Wrapper) DisableForwardingFromTunToDev(tunName string, devName string) error {
	if _, err := w.execWithBinary("-D", "FORWARD", "-i", tunName, "-o", devName, "-j", "ACCEPT"); err != nil {
		return fmt.Errorf(
			"failed to remove forwarding rule for %s -> %s: %v",
			tunName, devName, err)
	}

	return nil
}

func (w *Wrapper) EnableForwardingFromDevToTun(tunName string, devName string) error {
	if _, err := w.execWithBinary("-A", "FORWARD", "-i", devName, "-o", tunName, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("failed to set up forwarding rule for %s -> %s: %v",
			devName, tunName, err)
	}

	return nil
}

func (w *Wrapper) DisableForwardingFromDevToTun(tunName string, devName string) error {
	if _, err := w.execWithBinary("-D", "FORWARD", "-i", devName, "-o", tunName, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("failed to remove forwarding rule for %s -> %s: %v",
			devName, tunName, err)
	}

	return nil
}

func (w *Wrapper) ConfigureMssClamping() error {
	// Configuration for chain FORWARD
	if _, errForward := w.execWithBinary("-t", "mangle", "-A", "FORWARD", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"); errForward != nil {
		return fmt.Errorf("failed to configure MSS clamping on FORWARD chain: %s",
			errForward)
	}

	// Configuration for chain OUTPUT
	if _, errOutput := w.execWithBinary("-t", "mangle", "-A", "OUTPUT", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"); errOutput != nil {
		return fmt.Errorf("failed to configure MSS clamping on OUTPUT chain: %s",
			errOutput)
	}

	return nil
}

func (w *Wrapper) execWithBinary(args ...string) ([]byte, error) {
	var lastOut []byte
	var errs []error

	runOne := func(bin, label string) {
		if bin == "" || w.canBeSkipped(bin, args...) {
			return
		}
		out, err := w.commander.CombinedOutput(bin, args...)
		if err != nil {
			errs = append(errs, fmt.Errorf("[%s] %s %v failed: %w; output: %s", label, bin, args, err, out))
			return
		}
		if len(out) > 0 {
			lastOut = out
		}
	}

	runOne(w.v4binary, "IPv4")
	runOne(w.v6binary, "IPv6")

	if len(errs) > 0 {
		// простой join без errors.Join
		msg := "multiple errors:"
		for _, e := range errs {
			msg += "\n - " + e.Error()
		}
		return lastOut, errors.New(msg)
	}
	return lastOut, nil
}

func (w *Wrapper) canBeSkipped(binary string, args ...string) bool {
	action := ""
	checkArgs := make([]string, len(args))
	for i, a := range args {
		switch a {
		case "-A", "-D":
			action = a
			checkArgs[i] = "-C"
		default:
			checkArgs[i] = a
		}
	}

	_, checkErr := w.commander.CombinedOutput(binary, checkArgs...)

	switch action {
	case "-A":
		// Rule present -> skip add
		return checkErr == nil
	case "-D":
		// Rule absent -> skip delete
		return checkErr != nil
	default:
		return false
	}
}
