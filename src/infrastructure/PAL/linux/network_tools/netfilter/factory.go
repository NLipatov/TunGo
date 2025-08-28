package netfilter

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"syscall"
	"tungo/infrastructure/PAL/linux/network_tools/netfilter/iptables"
	"tungo/infrastructure/PAL/linux/network_tools/netfilter/nftables"

	"tungo/application"
	"tungo/infrastructure/PAL"
)

type Factory struct {
	cmd   PAL.Commander
	probe Probe
}

func NewFactory(cmd PAL.Commander) *Factory {
	return &Factory{
		cmd:   cmd,
		probe: DefaultProbe{},
	}
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
		log.Print("netfilter driver: nftables")
		if b, err := nftables.New(); err == nil {
			return nftables.NewSyncDriver(b), nil
		}
	}

	// 2) iptables-legacy
	if v4bin, ok := f.hasBinaryWorks("iptables-legacy"); ok {
		log.Print("netfilter driver: iptables-legacy")
		v6bin, _ := f.detectIP6Companion(v4bin) // optional
		return iptables.New(v4bin, v6bin, f.cmd), nil
	}

	// 3) plain "iptables": accept only legacy build
	if mode, out, err := f.iptablesMode("iptables"); err == nil && mode == "legacy" {
		v6bin, _ := f.detectIP6Companion("iptables")
		log.Print("netfilter driver: iptables")
		return iptables.New("iptables", v6bin, f.cmd), nil
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
