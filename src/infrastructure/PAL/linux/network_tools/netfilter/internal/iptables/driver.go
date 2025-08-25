package iptables

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"tungo/infrastructure/PAL"
)

// Driver is a thin, idempotent driver for iptables/ip6tables binaries.
// It prefers DOCKER-USER chain when present and falls back to FORWARD.
type Driver struct {
	cmd  PAL.Commander
	ipt4 string // e.g. "iptables-legacy" or "iptables"
	ipt6 string // e.g. "ip6tables-legacy" or "ip6tables" (can be empty if not available)
}

// NewDriverWithBinaries lets the factory inject explicit binaries (recommended).
func NewDriverWithBinaries(cmd PAL.Commander, ipt4, ipt6 string) *Driver {
	if ipt4 == "" {
		ipt4 = "iptables"
	}
	// ipt6 may be empty (IPv6 ops will be skipped).
	return &Driver{cmd: cmd, ipt4: ipt4, ipt6: ipt6}
}

// -------------------- public API --------------------

func (d *Driver) EnableDevMasquerade(devName string) error {
	if devName == "" {
		return errors.New("dev name is empty")
	}
	// v4
	if err := d.addIfMissing(d.ipt4, "nat", "POSTROUTING",
		"-o", devName, "-j", "MASQUERADE"); err != nil {
		return fmt.Errorf("enable v4 masquerade on %s: %w", devName, err)
	}
	// v6 (optional)
	if d.ipt6 != "" {
		if err := d.addIfMissing(d.ipt6, "nat", "POSTROUTING",
			"-o", devName, "-j", "MASQUERADE"); err != nil {
			return fmt.Errorf("enable v6 masquerade on %s: %w", devName, err)
		}
	}
	return nil
}

func (d *Driver) DisableDevMasquerade(devName string) error {
	if devName == "" {
		return errors.New("dev name is empty")
	}
	_ = d.delIfPresent(d.ipt4, "nat", "POSTROUTING",
		"-o", devName, "-j", "MASQUERADE")
	if d.ipt6 != "" {
		_ = d.delIfPresent(d.ipt6, "nat", "POSTROUTING",
			"-o", devName, "-j", "MASQUERADE")
	}
	return nil
}

func (d *Driver) EnableForwardingFromTunToDev(tunName, devName string) error {
	if tunName == "" || devName == "" {
		return errors.New("iface name is empty")
	}
	chain := d.pickForwardChain(d.ipt4) // "DOCKER-USER" if exists, else "FORWARD"
	// v4: tun -> dev accept
	if err := d.addIfMissing(d.ipt4, "filter", chain,
		"-i", tunName, "-o", devName, "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("v4 forward %s->%s: %w", tunName, devName, err)
	}
	// v4: dev -> tun ESTABLISHED,RELATED accept
	if err := d.addEstablishedRule(d.ipt4, "filter", chain, devName, tunName); err != nil {
		return fmt.Errorf("v4 reverse forward %s->%s: %w", devName, tunName, err)
	}

	// v6 (optional)
	if d.ipt6 != "" {
		chain6 := d.pickForwardChain(d.ipt6)
		if err := d.addIfMissing(d.ipt6, "filter", chain6,
			"-i", tunName, "-o", devName, "-j", "ACCEPT"); err != nil {
			return fmt.Errorf("v6 forward %s->%s: %w", tunName, devName, err)
		}
		if err := d.addEstablishedRule(d.ipt6, "filter", chain6, devName, tunName); err != nil {
			return fmt.Errorf("v6 reverse forward %s->%s: %w", devName, tunName, err)
		}
	}
	return nil
}

func (d *Driver) DisableForwardingFromTunToDev(tunName, devName string) error {
	if tunName == "" || devName == "" {
		return errors.New("iface name is empty")
	}
	chain := d.pickForwardChain(d.ipt4)
	_ = d.delIfPresent(d.ipt4, "filter", chain,
		"-i", tunName, "-o", devName, "-j", "ACCEPT")
	_ = d.delEstablishedRuleIfPresent(d.ipt4, "filter", chain, devName, tunName)

	if d.ipt6 != "" {
		chain6 := d.pickForwardChain(d.ipt6)
		_ = d.delIfPresent(d.ipt6, "filter", chain6,
			"-i", tunName, "-o", devName, "-j", "ACCEPT")
		_ = d.delEstablishedRuleIfPresent(d.ipt6, "filter", chain6, devName, tunName)
	}
	return nil
}

func (d *Driver) EnableForwardingFromDevToTun(tunName, devName string) error {
	// The pair rule is already installed in EnableForwardingFromTunToDev.
	return d.EnableForwardingFromTunToDev(tunName, devName)
}
func (d *Driver) DisableForwardingFromDevToTun(tunName, devName string) error {
	return d.DisableForwardingFromTunToDev(tunName, devName)
}

func (d *Driver) ConfigureMssClamping() error {
	// IPv4
	if err := d.addIfMissing(d.ipt4, "mangle", "FORWARD",
		"-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"); err != nil {
		return fmt.Errorf("v4 MSS clamp FORWARD: %w", err)
	}
	if err := d.addIfMissing(d.ipt4, "mangle", "OUTPUT",
		"-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"); err != nil {
		return fmt.Errorf("v4 MSS clamp OUTPUT: %w", err)
	}
	// IPv6 (optional)
	if d.ipt6 != "" {
		if err := d.addIfMissing(d.ipt6, "mangle", "FORWARD",
			"-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"); err != nil {
			return fmt.Errorf("v6 MSS clamp FORWARD: %w", err)
		}
		if err := d.addIfMissing(d.ipt6, "mangle", "OUTPUT",
			"-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"); err != nil {
			return fmt.Errorf("v6 MSS clamp OUTPUT: %w", err)
		}
	}
	return nil
}

// -------------------- internals --------------------

func (d *Driver) pickForwardChain(bin string) string {
	if d.chainExists(bin, "filter", "DOCKER-USER") {
		return "DOCKER-USER"
	}
	return "FORWARD"
}

func (d *Driver) addEstablishedRule(bin, table, chain, iif, oif string) error {
	// Prefer -m conntrack --ctstate; if not supported, fall back to -m state --state.
	specConntrack := []string{"-i", iif, "-o", oif, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"}
	if err := d.addIfMissing(bin, table, chain, specConntrack...); err == nil {
		return nil
	}
	// try legacy matcher
	specState := []string{"-i", iif, "-o", oif, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"}
	return d.addIfMissing(bin, table, chain, specState...)
}

func (d *Driver) delEstablishedRuleIfPresent(bin, table, chain, iif, oif string) error {
	specConntrack := []string{"-i", iif, "-o", oif, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"}
	if err := d.delIfPresent(bin, table, chain, specConntrack...); err == nil {
		return nil
	}
	specState := []string{"-i", iif, "-o", oif, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"}
	return d.delIfPresent(bin, table, chain, specState...)
}

func (d *Driver) chainExists(bin, table, chain string) bool {
	_, err := d.exec(bin, "-t", table, "-nL", chain)
	return err == nil
}

func (d *Driver) ruleExists(bin, table, chain string, spec ...string) bool {
	args := append([]string{"-t", table, "-C", chain}, spec...)
	_, err := d.exec(bin, args...)
	return err == nil
}

func (d *Driver) addIfMissing(bin, table, chain string, spec ...string) error {
	if d.ruleExists(bin, table, chain, spec...) {
		return nil
	}
	args := append([]string{"-t", table, "-A", chain}, spec...)
	if _, err := d.exec(bin, args...); err != nil {
		return fmt.Errorf("%s %v: %w", bin, strings.Join(args, " "), err)
	}
	return nil
}

func (d *Driver) delIfPresent(bin, table, chain string, spec ...string) error {
	if !d.ruleExists(bin, table, chain, spec...) {
		return nil
	}
	args := append([]string{"-t", table, "-D", chain}, spec...)
	if _, err := d.exec(bin, args...); err != nil {
		return fmt.Errorf("%s %v: %w", bin, strings.Join(args, " "), err)
	}
	return nil
}

func (d *Driver) exec(bin string, args ...string) ([]byte, error) {
	out, err := d.cmd.CombinedOutput(bin, args...)
	// Hide noisy empty outputs; keep actual stderr text for diagnostics.
	if err != nil {
		return out, fmt.Errorf("%s %s: %v, out: %s", bin, strings.Join(args, " "), err, bytes.TrimSpace(out))
	}
	return out, nil
}
