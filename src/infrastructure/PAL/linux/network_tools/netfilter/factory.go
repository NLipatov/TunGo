package netfilter

import (
	"bytes"
	"errors"
	"strings"
	"syscall"

	"tungo/application"
	"tungo/infrastructure/PAL"
	"tungo/infrastructure/PAL/linux/network_tools/netfilter/internal/interfaces/iptables"
	"tungo/infrastructure/PAL/linux/network_tools/netfilter/internal/interfaces/nftables"

	nftlib "github.com/google/nftables"
)

// Factory builds a concrete application.Netfilter implementation in OOP style.
type Factory struct {
	cmd PAL.Commander

	// Injected constructors (test-friendly).
	newNFT func() (application.Netfilter, error)
	// v4bin is required, v6bin may be empty if missing.
	newIPT func(v4bin, v6bin string) application.Netfilter
}

func NewFactory(cmd PAL.Commander) *Factory {
	return &Factory{
		cmd:    cmd,
		newNFT: func() (application.Netfilter, error) { return nftables.NewBackend() },
		newIPT: func(v4bin, v6bin string) application.Netfilter {
			return iptables.NewWrapperWithBinaries(cmd, v4bin, v6bin)
		},
	}
}

func (f *Factory) WithNFTConstructor(fn func() (application.Netfilter, error)) *Factory {
	if fn != nil {
		f.newNFT = fn
	}
	return f
}

func (f *Factory) WithIPTablesConstructor(fn func(v4bin, v6bin string) application.Netfilter) *Factory {
	if fn != nil {
		f.newIPT = fn
	}
	return f
}

// Build picks the best backend: nftables → iptables-legacy → (reject nf_tables iptables if nft unusable).
func (f *Factory) Build() (application.Netfilter, error) {
	// 1) nftables via netlink
	if ok, _ := f.kernelSupportsNFT(); ok {
		if b, err := f.newNFT(); err == nil {
			return b, nil
		}
	}

	// 2) iptables-legacy
	if v4bin, ok := f.hasBinaryWorks("iptables-legacy"); ok {
		v6bin, _ := f.detectIP6Companion(v4bin) // optional
		return f.newIPT(v4bin, v6bin), nil
	}

	// 3) plain "iptables": accept only legacy build
	if mode, out, err := f.iptablesMode("iptables"); err == nil && mode == "legacy" {
		v6bin, _ := f.detectIP6Companion("iptables")
		return f.newIPT("iptables", v6bin), nil
	} else if err == nil && mode == "nf_tables" {
		return nil, errors.New("iptables is (nf_tables) but nftables is unavailable; install iptables-legacy or enable nf_tables in the kernel")
	} else if err != nil && f.looksLikeNFTButKernelLacksSupport(err, out) {
		return nil, errors.New("iptables uses nft shim but kernel reports 'Protocol not supported'; install iptables-legacy or enable nf_tables")
	}

	return nil, errors.New("no netfilter backend available: install nftables (kernel+userspace) or iptables-legacy")
}

// ---------- helpers (instance methods) ----------

func (f *Factory) kernelSupportsNFT() (bool, error) {
	c, err := nftlib.New()
	if err != nil {
		return false, err
	}
	_ = c.CloseLasting()
	if _, err := c.ListTables(); err != nil {
		return false, err
	}
	return true, nil
}

func (f *Factory) hasBinaryWorks(bin string) (string, bool) {
	if out, err := f.cmd.CombinedOutput(bin, "-V"); err == nil && len(out) > 0 {
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
	if out, err := f.cmd.CombinedOutput(v6cand, "-V"); err == nil && len(out) > 0 {
		return v6cand, true
	}
	return "", false
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
			errors.Is(err, syscall.EOPNOTSUPP))
}
