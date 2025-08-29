package iptables

import (
	"errors"
	"fmt"
	"strings"
	"tungo/infrastructure/PAL"
)

type Chains struct {
	v4bin, v6bin string
	cmd          PAL.Commander
	wait         WaitPolicy
}

func NewChains(v4, v6 string, cmd PAL.Commander, wait WaitPolicy) *Chains {
	return &Chains{v4bin: v4, v6bin: v6, cmd: cmd, wait: wait}
}

func (c *Chains) EnsureAll(fwd, mangle string) error {
	// ensure chains
	if err := c.ensureChain("IPv4", c.v4bin, "filter", fwd); err != nil {
		return err
	}
	if err := c.ensureChain("IPv6", c.v6bin, "filter", fwd); err != nil {
		return err
	}
	if err := c.ensureChain("IPv4", c.v4bin, "mangle", mangle); err != nil {
		return err
	}
	if err := c.ensureChain("IPv6", c.v6bin, "mangle", mangle); err != nil {
		return err
	}

	// hooks (append; order-neutral)
	if err := c.ensureHookAppend("IPv4", c.v4bin, "filter", "FORWARD", fwd); err != nil {
		return err
	}
	if err := c.ensureHookAppend("IPv6", c.v6bin, "filter", "FORWARD", fwd); err != nil {
		return err
	}
	if err := c.ensureHookAppend("IPv4", c.v4bin, "mangle", "FORWARD", mangle); err != nil {
		return err
	}
	if err := c.ensureHookAppend("IPv6", c.v6bin, "mangle", "FORWARD", mangle); err != nil {
		return err
	}
	return nil
}

func (c *Chains) Teardown() error {
	var errs []error
	// unhook all duplicates
	if err := c.unhookAll("IPv4", c.v4bin, "filter", "FORWARD", fwdChain); err != nil {
		errs = append(errs, err)
	}
	if err := c.unhookAll("IPv6", c.v6bin, "filter", "FORWARD", fwdChain); err != nil {
		errs = append(errs, err)
	}
	if err := c.unhookAll("IPv4", c.v4bin, "mangle", "FORWARD", mangleChain); err != nil {
		errs = append(errs, err)
	}
	if err := c.unhookAll("IPv6", c.v6bin, "mangle", "FORWARD", mangleChain); err != nil {
		errs = append(errs, err)
	}

	// drop chains
	if err := c.dropChain("IPv4", c.v4bin, "filter", fwdChain); err != nil {
		errs = append(errs, err)
	}
	if err := c.dropChain("IPv6", c.v6bin, "filter", fwdChain); err != nil {
		errs = append(errs, err)
	}
	if err := c.dropChain("IPv4", c.v4bin, "mangle", mangleChain); err != nil {
		errs = append(errs, err)
	}
	if err := c.dropChain("IPv6", c.v6bin, "mangle", mangleChain); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errors.Join(errs...) // small local helper or use errors.Join in this file
	}
	return nil
}

// --- private ---

func (c *Chains) ensureChain(label, bin, table, chain string) error {
	if bin == "" {
		return nil
	}
	if _, err := c.cmd.CombinedOutput(bin, append(c.wait.Args(label), "-t", table, "-S", chain)...); err == nil {
		return nil
	}
	cmd := append(c.wait.Args(label), "-t", table, "-N", chain)
	out, err := c.cmd.CombinedOutput(bin, cmd...)
	if err != nil {
		o := strings.ToLower(string(out))
		if strings.Contains(o, "chain already exists") {
			return nil
		}
		return fmt.Errorf("[%s] create chain %s/%s failed: %w; output: %s", label, table, chain, err, out)
	}
	return nil
}

func (c *Chains) ensureHookAppend(label, bin, table, parent, child string) error {
	if bin == "" {
		return nil
	}
	if _, err := c.cmd.CombinedOutput(bin, append(c.wait.Args(label), "-t", table, "-C", parent, "-j", child)...); err == nil {
		return nil
	}
	cmd := append(c.wait.Args(label), "-t", table, "-A", parent, "-j", child)
	if out, err := c.cmd.CombinedOutput(bin, cmd...); err != nil {
		return fmt.Errorf("[%s] hook %s -> %s in %s failed: %w; output: %s", label, parent, child, table, err, out)
	}
	return nil
}

func (c *Chains) unhookAll(label, bin, table, parent, child string) error {
	if bin == "" {
		return nil
	}
	for {
		cmd := append(c.wait.Args(label), "-t", table, "-D", parent, "-j", child)
		out, err := c.cmd.CombinedOutput(bin, cmd...)
		if err != nil {
			if c.noSuchErr(strings.ToLower(string(out))) {
				return nil
			}
			return fmt.Errorf("[%s] unhook %s -> %s in %s failed: %w; output: %s", label, parent, child, table, err, out)
		}
	}
}

func (c *Chains) dropChain(label, bin, table, chain string) error {
	if bin == "" {
		return nil
	}
	_, _ = c.cmd.CombinedOutput(bin, append(c.wait.Args(label), "-t", table, "-F", chain)...)
	cmd := append(c.wait.Args(label), "-t", table, "-X", chain)
	out, err := c.cmd.CombinedOutput(bin, cmd...)
	if err != nil && !c.noSuchErr(string(out)) {
		return fmt.Errorf("[%s] delete chain %s/%s failed: %w; output: %s", label, table, chain, err, out)
	}
	return nil
}

func (c *Chains) noSuchErr(out string) bool {
	o := strings.ToLower(out)
	return strings.Contains(o, "no chain/target/match by that name") ||
		strings.Contains(o, "bad rule (does a matching rule exist") ||
		strings.Contains(o, "does a matching rule exist") ||
		strings.Contains(o, "no such file or directory")
}
