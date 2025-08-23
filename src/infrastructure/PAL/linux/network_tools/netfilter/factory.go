package netfilter

import (
	"bytes"
	"errors"
	"fmt"
	"tungo/application"
	"tungo/infrastructure/PAL"
	"tungo/infrastructure/PAL/linux/network_tools/netfilter/interfaces/nftables"

	gnft "github.com/google/nftables"
)

type BackendKind int

const (
	FWUnknown        BackendKind = iota
	FWNftables                   // native nft via netlink (github.com/google/nftables)
	FWIptablesLegacy             // classic xtables (iptables-legacy)
)

// String for logs
func (k BackendKind) String() string {
	switch k {
	case FWNftables:
		return "nftables"
	case FWIptablesLegacy:
		return "iptables(legacy)"
	default:
		return "unknown"
	}
}

// DetectResult tells which backend to use, and which iptables binaries (if legacy).
type DetectResult struct {
	Kind    BackendKind
	Reason  string // human-friendly explanation of the decision
	IPTBin  string // "iptables-legacy" or "iptables" (when compiled in legacy mode); empty for nft
	IP6Bin  string // "ip6tables-legacy" or "ip6tables" (legacy mode) if available; empty if not found
	ErrHint error  // optional extra hint error (e.g., nft shim present but kernel lacks nf_tables)
}

// NewAutoNetfilter detects the best available backend and returns a ready Netfilter.
// Preference order: nftables (netlink) → iptables-legacy → iptables(legacy).
// It never picks iptables(nf_tables) if nftables is unusable (common on Alpine
// kernels without nf_tables).
func NewAutoNetfilter(cmd PAL.Commander) (application.Netfilter, DetectResult, error) {
	// 1) Try nftables via netlink.
	if ok, reason, err := kernelSupportsNFT(); ok {
		b, err := nftables.NewBackend() // uses github.com/google/nftables
		if err != nil {
			// Extremely rare: netlink available but open failed.
			return nil, DetectResult{}, fmt.Errorf("nftables backend init failed: %w", err)
		}
		return b, DetectResult{Kind: FWNftables}, nil
	} else if err != nil {
		// keep going, but remember why nft failed (useful for final error)
		_ = reason
	}

	// 2) Try iptables-legacy explicitly.
	if bin, ok := hasBinaryVersionOK(cmd, "iptables-legacy"); ok {
		dr := DetectResult{Kind: FWIptablesLegacy, IPTBin: bin}

		// Optional IPv6 companion — if missing, IPv6 features in iptables backend may fail.
		if bin6, ok6 := hasBinaryVersionOK(cmd, "ip6tables-legacy"); ok6 {
			dr.IP6Bin = bin6
		}

		// Your current iptables backend constructor takes only IPv4 bin; it calls ip6tables directly.
		// If you later add a separate IPv6 binary field — pass dr.IP6Bin there as well.
		return NewIptables(cmd), dr, nil
	}

	// 3) Fallback to plain "iptables". Must ensure it's actually legacy, not nf_tables.
	if out, err := cmd.CombinedOutput("iptables", "-V"); err == nil {
		if bytes.Contains(out, []byte("(legacy)")) {
			dr := DetectResult{Kind: FWNftables, IPTBin: "iptables"}
			// Optional IPv6 check
			if out6, err6 := cmd.CombinedOutput("ip6tables", "-V"); err6 == nil && bytes.Contains(out6, []byte("(legacy)")) {
				dr.IP6Bin = "ip6tables"
			}
			return NewIptables(cmd), dr, nil
		}
		// Compiled as nf_tables. If nft usable — мы бы выбрали его выше; раз нет — это тупик.
		if bytes.Contains(out, []byte("(nf_tables)")) {
			return nil, DetectResult{}, errors.New(
				"iptables is (nf_tables) but nftables is unavailable; install iptables-legacy or enable nf_tables in kernel",
			)
		}
	}

	// Nothing usable found.
	return nil, DetectResult{}, errors.New(
		"no firewall backend available: install nftables (kernel+userspace) or iptables-legacy",
	)
}

// hasBinaryVersionOK checks that a binary exists in PATH and returns its name if `-V` works.
// We don't strictly parse the version output here except existence/success.
func hasBinaryVersionOK(cmd PAL.Commander, bin string) (string, bool) {
	if out, err := cmd.CombinedOutput(bin, "-V"); err == nil && len(out) > 0 {
		return bin, true
	}
	return "", false
}
func kernelSupportsNFT() (ok bool, reason string, errHint error) {
	c, err := gnft.New() // not lasting by default
	if err != nil {
		return false, "nftables netlink init failed", err
	}
	// CloseLasting is safe even for non-lasting conns (no-op).
	defer func(c *gnft.Conn) {
		_ = c.CloseLasting()
	}(c)

	if _, err := c.ListTables(); err != nil {
		return false, "nftables list tables failed", err
	}
	return true, "nftables available via netlink", nil
}
