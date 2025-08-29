package iptables

import (
	"fmt"
	"sync"
	"sync/atomic"
	"tungo/infrastructure/PAL"
)

const (
	fwdChain    = "IPTABLES-TUNGO-FWD"
	mangleChain = "IPTABLES-TUNGO-MANGLE"
)

type Driver struct {
	commander PAL.Commander
	v4bin     string
	v6bin     string

	chainsReady atomic.Bool
	initMu      sync.Mutex

	wait   *DefaultWaitPolicy
	skip   *DefaultSkipper
	exec   *FamilyExec
	chains *Chains
}

func New(v4bin, v6bin string, cmd PAL.Commander) *Driver {
	wait := NewWaitPolicy(v4bin, v6bin, cmd)
	skip := NewSkipper(v6bin, wait, cmd)
	exec := NewFamilyExec(v4bin, v6bin, cmd, wait, skip)
	chains := NewChains(v4bin, v6bin, cmd, wait)

	return &Driver{
		commander: cmd, v4bin: v4bin, v6bin: v6bin,
		wait: wait, skip: skip, exec: exec, chains: chains,
	}
}

// ----- public API -----

func (w *Driver) EnableDevMasquerade(dev string) error {
	args := []string{"-t", "nat", "-A", "POSTROUTING", "-o", dev, "-m", "comment", "--comment", "tungo", "-j", "MASQUERADE"}
	return w.execute(args...)
}

func (w *Driver) DisableDevMasquerade(dev string) error {
	args := []string{"-t", "nat", "-D", "POSTROUTING", "-o", dev, "-m", "comment", "--comment", "tungo", "-j", "MASQUERADE"}
	return w.execute(args...)
}

func (w *Driver) EnableForwardingFromTunToDev(tun, dev string) error {
	args := []string{"-t", "filter", "-A", fwdChain, "-i", tun, "-o", dev, "-m", "comment", "--comment", "tungo", "-j", "ACCEPT"}
	return w.execute(args...)
}

func (w *Driver) DisableForwardingFromTunToDev(tun, dev string) error {
	args := []string{"-t", "filter", "-D", fwdChain, "-i", tun, "-o", dev, "-m", "comment", "--comment", "tungo", "-j", "ACCEPT"}
	return w.execute(args...)
}

func (w *Driver) EnableForwardingFromDevToTun(tun, dev string) error {
	args := []string{"-t", "filter", "-A", fwdChain, "-i", dev, "-o", tun,
		"-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED",
		"-m", "comment", "--comment", "tungo", "-j", "ACCEPT"}
	return w.execute(args...)
}

func (w *Driver) DisableForwardingFromDevToTun(tun, dev string) error {
	args := []string{"-t", "filter", "-D", fwdChain, "-i", dev, "-o", tun,
		"-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED",
		"-m", "comment", "--comment", "tungo", "-j", "ACCEPT"}
	return w.execute(args...)
}

func (w *Driver) ConfigureMssClamping(dev string) error {
	out := []string{"-t", "mangle", "-A", mangleChain, "-o", dev, "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN",
		"-m", "comment", "--comment", "tungo", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}
	if err := w.execute(out...); err != nil {
		return err
	}
	in := []string{"-t", "mangle", "-A", mangleChain, "-i", dev, "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN",
		"-m", "comment", "--comment", "tungo", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}
	return w.execute(in...)
}

func (w *Driver) TeardownChains() error { return w.chains.Teardown() }

func (w *Driver) execute(base ...string) error {
	if !w.chainsReady.Load() {
		w.initMu.Lock()
		if !w.chainsReady.Load() {
			if err := w.chains.EnsureAll(fwdChain, mangleChain); err != nil {
				w.initMu.Unlock()
				return fmt.Errorf("ensure chains: %w", err)
			}
			w.chainsReady.Store(true)
		}
		w.initMu.Unlock()
	}
	table := w.getTableFromArgs(base)
	return w.exec.ExecBothFamilies(base, table, table == "nat")
}

func (w *Driver) getTableFromArgs(args []string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-t" {
			return args[i+1]
		}
	}
	return "filter"
}
