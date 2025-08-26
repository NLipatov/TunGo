// Package nftables provides a robust Netfilter backend using google/nftables.
//
// Design highlights:
//   - Pure netlink (no shell-out), locale-agnostic.
//   - Own tables/namespaces (tungo_nat/tungo_filter) to avoid clobbering system rules.
//   - Idempotency via Rule.UserData tags (stable, not handle- or text-based).
//   - Forward base chain priority -100 to precede typical filter(0)/policy drop.
//   - IPv6 is attempted and silently skipped if the address family is unsupported.
//   - Clear diagnostics for ip_forward=0 and missing nf_conntrack.
//
// NOTE: MSS clamping is intentionally not implemented here. Prefer sysctl
// net.ipv4.tcp_mtu_probing=1; nft exprs for clamping are non-trivial and brittle.

package nftables

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"syscall"

	nft "github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"

	"tungo/application" // your interface package
)

// Compile-time check against your domain contract.
var _ application.Netfilter = (*Nftables)(nil)

// Config controls backend behavior. Zero value is sane.
type Config struct {
	// Names and priorities. Change only if you need to integrate with
	// existing rulesets in a controlled environment.
	TableNat4Name        string // family ip
	TableNat6Name        string // family ip6 (name reuse across families is fine)
	TableInetName        string // family inet
	PostroutingChainName string
	ForwardChainName     string

	// Base-chain priorities (nft "priority"). Lower runs earlier.
	// Default: NAT src at 100, filter forward at -100.
	PrioritySrcNAT  int
	PriorityForward int

	// If true, return an error when /proc/sys/net/ipv4/ip_forward == 0.
	// Otherwise, proceed but rules won't take effect until it's enabled.
	EnforceIPv4Forwarding bool
}

// DefaultConfig returns the tuned defaults.
func DefaultConfig() Config {
	return Config{
		TableNat4Name:         "tungo_nat",
		TableNat6Name:         "tungo_nat",
		TableInetName:         "tungo_filter",
		PostroutingChainName:  "postrouting",
		ForwardChainName:      "forward",
		PrioritySrcNAT:        100,
		PriorityForward:       -100,
		EnforceIPv4Forwarding: true,
	}
}

// Nftables is a stateful nftables-backed netfilter implementation.
type Nftables struct {
	conn conn
	cfg  Config
}

// NewNetfilter creates a lasting netlink connection. Requires CAP_NET_ADMIN.
func NewNetfilter() (*Nftables, error) { return NewNetfilterWithConfig(DefaultConfig()) }

// NewNetfilterWithConfig allows fine-tuning behavior.
func NewNetfilterWithConfig(cfg Config) (*Nftables, error) {
	c, err := nft.New(nft.AsLasting())
	if err != nil {
		return nil, fmt.Errorf("nftables conn: %w", err)
	}
	return &Nftables{conn: c, cfg: cfg}, nil
}

func NewNetfilterWithConfigAndConn(conn conn, cfg Config) (*Nftables, error) {
	return &Nftables{conn: conn, cfg: cfg}, nil
}

// Close shuts down the lasting netlink connection.
func (b *Nftables) Close() error {
	if b == nil || b.conn == nil {
		return nil
	}
	return b.conn.CloseLasting()
}

// ---------- internal helpers: sysctl / feature detection ----------

func readSysctl(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func isAFNotSupported(err error) bool {
	s := strings.ToLower(err.Error())
	return errors.Is(err, syscall.EAFNOSUPPORT) ||
		strings.Contains(s, "address family not supported")
}

func isConntrackMissing(err error) bool {
	s := strings.ToLower(err.Error())
	return errors.Is(err, syscall.EOPNOTSUPP) ||
		strings.Contains(s, "operation not supported") ||
		strings.Contains(s, "conntrack")
}

// ---------- internal helpers: ensure tables/chains (idempotent) ----------

func (b *Nftables) ensureTableFlushed(fam nft.TableFamily, name string) (*nft.Table, bool, error) {
	tables, err := b.conn.ListTables()
	if err != nil {
		return nil, false, fmt.Errorf("list tables: %w", err)
	}
	for _, t := range tables {
		if t.Family == fam && t.Name == name {
			return t, false, nil
		}
	}
	t := &nft.Table{Family: fam, Name: name}
	b.conn.AddTable(t)
	if err := b.conn.Flush(); err != nil {
		if isAFNotSupported(err) {
			// Let caller decide how to handle (e.g., skip IPv6 quietly).
			return nil, false, err
		}
		return nil, false, fmt.Errorf("add table %v/%s: %w", fam, name, err)
	}
	return t, true, nil
}

func (b *Nftables) ensureBaseChainFlushed(
	t *nft.Table,
	name string,
	ctype nft.ChainType,
	hook nft.ChainHook,
	prio int,
) (*nft.Chain, bool, error) {
	chains, err := b.conn.ListChains()
	if err != nil {
		return nil, false, fmt.Errorf("list chains: %w", err)
	}
	for _, ch := range chains {
		if ch.Table != nil && ch.Table.Name == t.Name && ch.Table.Family == t.Family && ch.Name == name {
			return ch, false, nil
		}
	}
	ct := ctype
	hk := hook
	p := nft.ChainPriority(prio)

	ch := &nft.Chain{
		Table:    t,
		Name:     name,
		Type:     ct,  // base chain
		Hooknum:  &hk, // pointer required by API
		Priority: &p,  // pointer required by API
	}
	if ctype == nft.ChainTypeFilter {
		pol := nft.ChainPolicyAccept
		ch.Policy = &pol
	}
	b.conn.AddChain(ch)
	if err := b.conn.Flush(); err != nil {
		return nil, false, fmt.Errorf("add chain %s/%s: %w", t.Name, name, err)
	}
	return ch, true, nil
}

// getChain looks up an existing table/chain without creating anything.
func (b *Nftables) getChain(fam nft.TableFamily, tableName, chainName string) (*nft.Table, *nft.Chain, error) {
	tables, err := b.conn.ListTables()
	if err != nil {
		return nil, nil, fmt.Errorf("list tables: %w", err)
	}
	var tbl *nft.Table
	for _, t := range tables {
		if t.Family == fam && t.Name == tableName {
			tbl = t
			break
		}
	}
	if tbl == nil {
		return nil, nil, os.ErrNotExist
	}
	chains, err := b.conn.ListChains()
	if err != nil {
		return nil, nil, fmt.Errorf("list chains: %w", err)
	}
	for _, ch := range chains {
		if ch.Table != nil && ch.Table.Family == fam && ch.Table.Name == tableName && ch.Name == chainName {
			return tbl, ch, nil
		}
	}
	return tbl, nil, os.ErrNotExist
}

// ---------- internal helpers: expr builders ----------

// nft string operands are NUL-terminated.
func zstr(s string) []byte { return append([]byte(s), 0x00) }

func exprMasqueradeForOIF(dev string) []expr.Any {
	return []expr.Any{
		&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: zstr(dev)},
		&expr.Masq{},
	}
}

func exprForwardAcceptIIFtoOIF(iif, oif string) []expr.Any {
	return []expr.Any{
		&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: zstr(iif)},
		&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: zstr(oif)},
		&expr.Verdict{Kind: expr.VerdictAccept},
	}
}

func exprForwardAcceptEstablished(iif, oif string) []expr.Any {
	// ct state & (established|related) != 0
	mask := binaryutil.BigEndian.PutUint32(expr.CtStateBitESTABLISHED | expr.CtStateBitRELATED)
	return []expr.Any{
		&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: zstr(iif)},
		&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: zstr(oif)},
		&expr.Ct{Register: 1, Key: expr.CtKeySTATE},
		&expr.Bitwise{SourceRegister: 1, DestRegister: 1, Len: 4, Mask: mask, Xor: []byte{0, 0, 0, 0}},
		&expr.Cmp{Op: expr.CmpOpNeq, Register: 1, Data: []byte{0, 0, 0, 0}},
		&expr.Verdict{Kind: expr.VerdictAccept},
	}
}

// ---------- internal helpers: idempotent add/del by UserData tag ----------

func (b *Nftables) addIfMissingByTag(t *nft.Table, ch *nft.Chain, e []expr.Any, tag []byte) error {
	rules, err := b.conn.GetRules(t, ch)
	if err != nil {
		return fmt.Errorf("get rules %s/%s: %w", t.Name, ch.Name, err)
	}
	for _, r := range rules {
		if reflect.DeepEqual(r.UserData, tag) {
			return nil
		}
	}
	b.conn.AddRule(&nft.Rule{Table: t, Chain: ch, Exprs: e, UserData: tag})
	return nil
}

func (b *Nftables) delIfPresentByTag(t *nft.Table, ch *nft.Chain, tag []byte) error {
	rules, err := b.conn.GetRules(t, ch)
	if err != nil {
		return fmt.Errorf("get rules %s/%s: %w", t.Name, ch.Name, err)
	}
	for _, r := range rules {
		if reflect.DeepEqual(r.UserData, tag) {
			_ = b.conn.DelRule(r)
			break
		}
	}
	return nil
}

// ========================= application.Netfilter ==============================

func (b *Nftables) EnableDevMasquerade(devName string) error {
	if devName == "" {
		return errors.New("dev name is empty")
	}

	// IPv4
	t4, _, err := b.ensureTableFlushed(nft.TableFamilyIPv4, b.cfg.TableNat4Name)
	if err != nil {
		return err
	}
	ch4, _, err := b.ensureBaseChainFlushed(t4, b.cfg.PostroutingChainName, nft.ChainTypeNAT, *nft.ChainHookPostrouting, b.cfg.PrioritySrcNAT)
	if err != nil {
		return err
	}
	if err := b.addIfMissingByTag(t4, ch4, exprMasqueradeForOIF(devName), []byte("tungo:masq4 oif="+devName)); err != nil {
		return err
	}

	// IPv6 (optional)
	if t6, _, err := b.ensureTableFlushed(nft.TableFamilyIPv6, b.cfg.TableNat6Name); err == nil {
		if ch6, _, err := b.ensureBaseChainFlushed(t6, b.cfg.PostroutingChainName, nft.ChainTypeNAT, *nft.ChainHookPostrouting, b.cfg.PrioritySrcNAT); err == nil {
			if err := b.addIfMissingByTag(t6, ch6, exprMasqueradeForOIF(devName), []byte("tungo:masq6 oif="+devName)); err != nil {
				return err
			}
		}
	}

	if err := b.conn.Flush(); err != nil {
		return fmt.Errorf("flush nat masquerade: %w", err)
	}
	return nil
}

func (b *Nftables) DisableDevMasquerade(devName string) error {
	if devName == "" {
		return errors.New("dev name is empty")
	}

	// IPv4
	if t4, _, err := b.ensureTableFlushed(nft.TableFamilyIPv4, b.cfg.TableNat4Name); err == nil {
		if ch4, _, err := b.ensureBaseChainFlushed(t4, b.cfg.PostroutingChainName, nft.ChainTypeNAT, *nft.ChainHookPostrouting, b.cfg.PrioritySrcNAT); err == nil {
			_ = b.delIfPresentByTag(t4, ch4, []byte("tungo:masq4 oif="+devName))
		}
	}

	// IPv6 (optional)
	if t6, _, err := b.ensureTableFlushed(nft.TableFamilyIPv6, b.cfg.TableNat6Name); err == nil {
		if ch6, _, err := b.ensureBaseChainFlushed(t6, b.cfg.PostroutingChainName, nft.ChainTypeNAT, *nft.ChainHookPostrouting, b.cfg.PrioritySrcNAT); err == nil {
			_ = b.delIfPresentByTag(t6, ch6, []byte("tungo:masq6 oif="+devName))
		}
	}

	if err := b.conn.Flush(); err != nil {
		return fmt.Errorf("flush nat unmasq: %w", err)
	}
	return nil
}

func (b *Nftables) EnableForwardingFromTunToDev(tunName, devName string) error {
	if tunName == "" || devName == "" {
		return errors.New("iface name is empty")
	}

	// Optional enforcement: ensure IPv4 forwarding is enabled.
	if b.cfg.EnforceIPv4Forwarding {
		if v, err := readSysctl("/proc/sys/net/ipv4/ip_forward"); err == nil && v == "0" {
			return errors.New("net.ipv4.ip_forward=0: enable packet forwarding (e.g., `sysctl -w net.ipv4.ip_forward=1`)")
		}
	}

	t, _, err := b.ensureTableFlushed(nft.TableFamilyINet, b.cfg.TableInetName)
	if err != nil {
		return err
	}
	ch, _, err := b.ensureBaseChainFlushed(t, b.cfg.ForwardChainName, nft.ChainTypeFilter, *nft.ChainHookForward, b.cfg.PriorityForward)
	if err != nil {
		return err
	}
	if err := b.addIfMissingByTag(t, ch, exprForwardAcceptIIFtoOIF(tunName, devName), []byte("tungo:fwd iif="+tunName+" oif="+devName)); err != nil {
		return err
	}
	if err := b.addIfMissingByTag(t, ch, exprForwardAcceptEstablished(devName, tunName), []byte("tungo:fwdret iif="+devName+" oif="+tunName)); err != nil {
		return err
	}

	if err := b.conn.Flush(); err != nil {
		if isConntrackMissing(err) {
			return fmt.Errorf("flush inet forward: %w (conntrack likely missing; load nf_conntrack)", err)
		}
		return fmt.Errorf("flush inet forward: %w", err)
	}
	return nil
}

func (b *Nftables) DisableForwardingFromTunToDev(tunName, devName string) error {
	if tunName == "" || devName == "" {
		return errors.New("iface name is empty")
	}

	if t, _, err := b.ensureTableFlushed(nft.TableFamilyINet, b.cfg.TableInetName); err == nil {
		if ch, _, err := b.ensureBaseChainFlushed(t, b.cfg.ForwardChainName, nft.ChainTypeFilter, *nft.ChainHookForward, b.cfg.PriorityForward); err == nil {
			_ = b.delIfPresentByTag(t, ch, []byte("tungo:fwd iif="+tunName+" oif="+devName))
			_ = b.delIfPresentByTag(t, ch, []byte("tungo:fwdret iif="+devName+" oif="+tunName))
		}
	}

	if err := b.conn.Flush(); err != nil {
		return fmt.Errorf("flush forward cleanup: %w", err)
	}
	return nil
}

func (b *Nftables) EnableForwardingFromDevToTun(tunName, devName string) error {
	return b.EnableForwardingFromTunToDev(tunName, devName)
}
func (b *Nftables) DisableForwardingFromDevToTun(tunName, devName string) error {
	return b.DisableForwardingFromTunToDev(tunName, devName)
}

// ConfigureMssClamping is intentionally not implemented here.
// Prefer: sysctl net.ipv4.tcp_mtu_probing=1 (system-wide, simpler, less brittle).
func (b *Nftables) ConfigureMssClamping() error {
	return errors.New("MSS clamping via nftables expressions is not implemented; prefer sysctl net.ipv4.tcp_mtu_probing=1")
}
