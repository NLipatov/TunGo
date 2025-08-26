package netfilter

import (
	"bytes"
	"errors"
	"strings"
	"syscall"
	"tungo/infrastructure/PAL/linux/network_tools/netfilter/iptables"
	"tungo/infrastructure/PAL/linux/network_tools/netfilter/nftables"

	"tungo/application"
	"tungo/infrastructure/PAL"
)

type Factory struct {
	cmd        PAL.Commander
	nftFactory nftables.Factory
	iptFactory iptables.Factory
	probe      Probe
}

func NewFactory(cmd PAL.Commander) *Factory {
	return &Factory{
		cmd:        cmd,
		nftFactory: nftables.DefaultFactory{},
		iptFactory: iptables.NewDefaultFactory(cmd),
		probe:      DefaultProbe{},
	}
}

// DI hooks (handy for tests)
func (f *Factory) WithNFTFactory(n nftables.Factory) *Factory {
	if n != nil {
		f.nftFactory = n
	}
	return f
}
func (f *Factory) WithIPTablesFactory(i iptables.Factory) *Factory {
	if i != nil {
		f.iptFactory = i
	}
	return f
}
func (f *Factory) WithProbe(p Probe) *Factory {
	if p != nil {
		f.probe = p
	}
	return f
}

// Build picks the best backend: nftables → iptables-legacy → accept iptables(legacy) but reject iptables(nf_tables) if nft is unusable.
func (f *Factory) Build() (application.Netfilter, error) {
	// 1) nftables via netlink (through probe object)
	if ok, _ := f.probe.Supports(); ok {
		if b, err := f.nftFactory.New(); err == nil {
			return b, nil
		}
	}

	// 2) iptables-legacy
	if v4bin, ok := f.hasBinaryWorks("iptables-legacy"); ok {
		v6bin, _ := f.detectIP6Companion(v4bin) // optional
		return f.iptFactory.New(v4bin, v6bin), nil
	}

	// 3) plain "iptables": accept only legacy build
	if mode, out, err := f.iptablesMode("iptables"); err == nil && mode == "legacy" {
		v6bin, _ := f.detectIP6Companion("iptables")
		return f.iptFactory.New("iptables", v6bin), nil
	} else if err == nil && mode == "nf_tables" {
		return nil, errors.New("iptables is (nf_tables) but nftables is unavailable; install iptables-legacy or enable nf_tables in the kernel")
	} else if err != nil && f.looksLikeNFTButKernelLacksSupport(err, out) {
		return nil, errors.New("iptables uses nft shim but kernel reports 'Protocol not supported'; install iptables-legacy or enable nf_tables")
	}

	return nil, errors.New("no netfilter backend available: install nftables (kernel+userspace) or iptables-legacy")
}

// ---------- helpers (instance methods) ----------

func (f *Factory) hasBinaryWorks(bin string) (string, bool) {
	if out, err := f.cmd.CombinedOutput(bin, "-V"); err == nil && len(bytes.TrimSpace(out)) > 0 {
		return bin, true
	}
	return "", false
}

// detectIP6Companion tries to find matching ip6 binary for the given v4 binary.
func (f *Factory) detectIP6Companion(v4bin string) (string, bool) {
	var v6cand string
	switch v4bin {
	case "iptables-legacy":
		v6cand = "ip6tables-legacy"
	case "iptables":
		v6cand = "ip6tables"
	default:
		return "", false
	}
	if _, ok := f.hasBinaryWorks(v6cand); !ok {
		return "", false
	}
	return v6cand, true
}

// iptablesMode returns "legacy", "nf_tables", or "" with raw output for diagnostics.
func (f *Factory) iptablesMode(bin string) (string, []byte, error) {
	out, err := f.cmd.CombinedOutput(bin, "-V")
	if err != nil {
		return "", out, err
	}
	switch {
	case bytes.Contains(out, []byte("(legacy)")):
		return "legacy", out, nil
	case bytes.Contains(out, []byte("(nf_tables)")):
		return "nf_tables", out, nil
	default:
		return "", out, nil
	}
}

func (f *Factory) looksLikeNFTButKernelLacksSupport(err error, out []byte) bool {
	msg := strings.ToLower(err.Error() + " " + string(out))
	return strings.Contains(msg, "failed to initialize nft") &&
		(strings.Contains(msg, "protocol not supported") ||
			strings.Contains(msg, "operation not supported") ||
			errors.Is(err, syscall.EOPNOTSUPP) ||
			errors.Is(err, syscall.EAFNOSUPPORT))
}
