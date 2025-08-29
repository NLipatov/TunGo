// Package nftables: a minimal 1:1 replacement for the simple iptables wrapper,
// implemented via github.com/google/nftables (no shell-outs, locale-agnostic).
//
// Exact semantics (mirror the provided iptables code):
//   - EnableDevMasquerade(dev):  ip nat POSTROUTING  -o dev  -j MASQUERADE  (append)
//   - DisableDevMasquerade(dev): ip nat POSTROUTING  -o dev  -j MASQUERADE  (delete)
//   - EnableForwardingFromTunToDev(tun, dev): ip filter FORWARD -i tun -o dev -j ACCEPT (append)
//   - DisableForwardingFromTunToDev(...):     delete the exact rule above
//   - EnableForwardingFromDevToTun(tun, dev): ip filter FORWARD -i dev -o tun -m state --state RELATED,ESTABLISHED -j ACCEPT (append)
//   - DisableForwardingFromDevToTun(...):     delete the exact rule above
//
// Notes:
//   - We write strictly into system tables/chains: ip/ip6 "nat/POSTROUTING" and
//     "filter/FORWARD". If a table/chain is missing (nft-only host w/o iptables),
//     we create it with the conventional base chain hook/priority (identical to iptables-nft).
//   - We DO NOT touch DOCKER-USER or create any custom filter tables.
//   - IPv6 is best-effort: skipped quietly if AF_INET6 unsupported.
//   - Reliability: serialized ops + retry&reconnect on "mismatched sequence" netlink errors.
//   - Idempotency: rules carry Rule.UserData tags; deletions match by tag.
//   - MSS clamping (TCPMSS) is NOT implemented (nft expr is brittle). Prefer sysctl:
//     sysctl -w net.ipv4.tcp_mtu_probing=1
package nftables

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"time"

	nft "github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
)

// Driver is drop-in compatible with your iptables wrapper's interface.
type Driver struct {
	mu   sync.Mutex
	conn *nft.Conn
	cfg  Config
}

type Config struct {
	// Netlink retry policy.
	MaxNetlinkRetries int           // default 3
	RetryBackoff      time.Duration // default 80ms

	// When creating missing base chains, use these priorities.
	// iptables-nft uses: nat postrouting prio 100, filter forward prio 0.
	NatPostroutingPrio int // default 100
	FilterForwardPrio  int // default 0

	// Optionally verify sysctl before writing FORWARD rules (not in iptables code, off by default).
	EnforceIPv4Forwarding bool // default false
}

func DefaultConfig() Config {
	return Config{
		MaxNetlinkRetries:     3,
		RetryBackoff:          80 * time.Millisecond,
		NatPostroutingPrio:    100,
		FilterForwardPrio:     0,
		EnforceIPv4Forwarding: false,
	}
}

func New() (*Driver, error) {
	return NewWithConfig(DefaultConfig())
}

func NewWithConfig(cfg Config) (*Driver, error) {
	c, err := nft.New(nft.AsLasting())
	if err != nil {
		return nil, fmt.Errorf("nftables conn: %w", err)
	}
	return &Driver{conn: c, cfg: cfg}, nil
}

func (d *Driver) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conn == nil {
		return nil
	}
	return d.conn.CloseLasting()
}

// -------------------- public API (1:1) --------------------

func (d *Driver) EnableDevMasquerade(devName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if devName == "" {
		return errors.New("dev name is empty")
	}
	return d.withRetry(func() error {
		t, ch, err := d.ensureSystemNatPostrouting(nft.TableFamilyIPv4)
		if err != nil {
			return err
		}
		if err := d.appendIfMissingByTag(t, ch, exprMasqOIF(devName), tagMasq4(devName)); err != nil {
			return err
		}
		// IPv6 best-effort
		if t6, ch6, err := d.ensureSystemNatPostrouting(nft.TableFamilyIPv6); err == nil {
			_ = d.appendIfMissingByTag(t6, ch6, exprMasqOIF(devName), tagMasq6(devName))
		} else if !isAFNotSupported(err) {
			return err
		}
		return d.conn.Flush()
	})
}

func (d *Driver) DisableDevMasquerade(devName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if devName == "" {
		return errors.New("dev name is empty")
	}
	return d.withRetry(func() error {
		// v4
		if t, ch, err := d.ensureSystemNatPostrouting(nft.TableFamilyIPv4); err == nil {
			_ = d.delIfPresentByTag(t, ch, tagMasq4(devName))
		} else if !isAFNotSupported(err) {
			return err
		}
		// v6
		if t6, ch6, err := d.ensureSystemNatPostrouting(nft.TableFamilyIPv6); err == nil {
			_ = d.delIfPresentByTag(t6, ch6, tagMasq6(devName))
		} else if !isAFNotSupported(err) {
			return err
		}
		return d.conn.Flush()
	})
}

func (d *Driver) EnableForwardingFromTunToDev(tunName, devName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if tunName == "" || devName == "" {
		return errors.New("iface name is empty")
	}
	return d.withRetry(func() error {
		if d.cfg.EnforceIPv4Forwarding {
			if v, err := readSysctl("/proc/sys/net/ipv4/ip_forward"); err == nil && v == "0" {
				return errors.New("net.ipv4.ip_forward=0")
			}
		}
		// v4
		t, ch, err := d.ensureSystemFilterForward(nft.TableFamilyIPv4)
		if err != nil {
			return err
		}
		if err := d.appendIfMissingByTag(t, ch, exprAcceptIIFtoOIF(tunName, devName), tagV4Fwd(tunName, devName)); err != nil {
			return err
		}
		// v6 best-effort
		if t6, ch6, err := d.ensureSystemFilterForward(nft.TableFamilyIPv6); err == nil {
			_ = d.appendIfMissingByTag(t6, ch6, exprAcceptIIFtoOIF(tunName, devName), tagV6Fwd(tunName, devName))
		} else if !isAFNotSupported(err) {
			return err
		}
		return d.conn.Flush()
	})
}

func (d *Driver) DisableForwardingFromTunToDev(tunName, devName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if tunName == "" || devName == "" {
		return errors.New("iface name is empty")
	}
	return d.withRetry(func() error {
		// v4
		if t, ch, err := d.ensureSystemFilterForward(nft.TableFamilyIPv4); err == nil {
			_ = d.delIfPresentByTag(t, ch, tagV4Fwd(tunName, devName))
		} else if !isAFNotSupported(err) {
			return err
		}
		// v6
		if t6, ch6, err := d.ensureSystemFilterForward(nft.TableFamilyIPv6); err == nil {
			_ = d.delIfPresentByTag(t6, ch6, tagV6Fwd(tunName, devName))
		} else if !isAFNotSupported(err) {
			return err
		}
		return d.conn.Flush()
	})
}

func (d *Driver) EnableForwardingFromDevToTun(tunName, devName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if tunName == "" || devName == "" {
		return errors.New("iface name is empty")
	}
	return d.withRetry(func() error {
		// v4
		t, ch, err := d.ensureSystemFilterForward(nft.TableFamilyIPv4)
		if err != nil {
			return err
		}
		if err := d.appendIfMissingByTag(t, ch, exprAcceptEstablished(devName, tunName), tagV4FwdRet(devName, tunName)); err != nil {
			return err
		}
		// v6 best-effort
		if t6, ch6, err := d.ensureSystemFilterForward(nft.TableFamilyIPv6); err == nil {
			_ = d.appendIfMissingByTag(t6, ch6, exprAcceptEstablished(devName, tunName), tagV6FwdRet(devName, tunName))
		} else if !isAFNotSupported(err) {
			return err
		}
		return d.conn.Flush()
	})
}

func (d *Driver) DisableForwardingFromDevToTun(tunName, devName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if tunName == "" || devName == "" {
		return errors.New("iface name is empty")
	}
	return d.withRetry(func() error {
		// v4
		if t, ch, err := d.ensureSystemFilterForward(nft.TableFamilyIPv4); err == nil {
			_ = d.delIfPresentByTag(t, ch, tagV4FwdRet(devName, tunName))
		} else if !isAFNotSupported(err) {
			return err
		}
		// v6
		if t6, ch6, err := d.ensureSystemFilterForward(nft.TableFamilyIPv6); err == nil {
			_ = d.delIfPresentByTag(t6, ch6, tagV6FwdRet(devName, tunName))
		} else if !isAFNotSupported(err) {
			return err
		}
		return d.conn.Flush()
	})
}

// 1:1 parity note: iptables wrapper adds MSS clamping rules here.
// nft equivalent is intentionally not implemented (prefer sysctl probing).
func (d *Driver) ConfigureMssClamping(_ string) error {
	return errors.New("MSS clamping via nft is not implemented; prefer sysctl net.ipv4.tcp_mtu_probing=1")
}

// -------------------- internals --------------------

func (d *Driver) withRetry(op func() error) error {
	var last error
	for i := 0; i < d.cfg.MaxNetlinkRetries; i++ {
		if i > 0 && d.cfg.RetryBackoff > 0 {
			time.Sleep(d.cfg.RetryBackoff)
		}
		if i > 0 {
			_ = d.resetConn()
		}
		if err := op(); err != nil {
			last = err
			if isSeqMismatch(err) {
				continue
			}
			return err
		}
		return nil
	}
	return last
}

func (d *Driver) resetConn() error {
	if d.conn != nil {
		_ = d.conn.CloseLasting()
	}
	c, err := nft.New(nft.AsLasting())
	if err != nil {
		return err
	}
	d.conn = c
	return nil
}

func (d *Driver) ensureSystemNatPostrouting(fam nft.TableFamily) (*nft.Table, *nft.Chain, error) {
	// Find or create "nat/POSTROUTING" like iptables-nft would.
	t, ch, err := d.getChain(fam, "nat", "POSTROUTING")
	if err == nil && ch != nil {
		return t, ch, nil
	}
	if isAFNotSupported(err) {
		return nil, nil, err
	}
	// Create the table if missing.
	if t == nil {
		t = &nft.Table{Family: fam, Name: "nat"}
		d.conn.AddTable(t)
		if e := d.conn.Flush(); e != nil {
			return nil, nil, fmt.Errorf("add table %v/nat: %w", fam, e)
		}
	}
	// Add base chain POSTROUTING (type NAT, hook postrouting, prio cfg.NatPostroutingPrio).
	h := *nft.ChainHookPostrouting
	p := nft.ChainPriority(d.cfg.NatPostroutingPrio)
	ch = &nft.Chain{Table: t, Name: "POSTROUTING", Type: nft.ChainTypeNAT, Hooknum: &h, Priority: &p}
	d.conn.AddChain(ch)
	if e := d.conn.Flush(); e != nil {
		return nil, nil, fmt.Errorf("add chain nat/POSTROUTING: %w", e)
	}
	return t, ch, nil
}

func (d *Driver) ensureSystemFilterForward(fam nft.TableFamily) (*nft.Table, *nft.Chain, error) {
	// Find or create "filter/FORWARD" like iptables-nft would.
	t, ch, err := d.getChain(fam, "filter", "FORWARD")
	if err == nil && ch != nil {
		return t, ch, nil
	}
	if isAFNotSupported(err) {
		return nil, nil, err
	}
	// Create table if missing.
	if t == nil {
		t = &nft.Table{Family: fam, Name: "filter"}
		d.conn.AddTable(t)
		if e := d.conn.Flush(); e != nil {
			return nil, nil, fmt.Errorf("add table %v/filter: %w", fam, e)
		}
	}
	// Add base chain FORWARD (type FILTER, hook forward, prio cfg.FilterForwardPrio, policy ACCEPT).
	h := *nft.ChainHookForward
	p := nft.ChainPriority(d.cfg.FilterForwardPrio)
	pol := nft.ChainPolicyAccept
	ch = &nft.Chain{Table: t, Name: "FORWARD", Type: nft.ChainTypeFilter, Hooknum: &h, Priority: &p, Policy: &pol}
	d.conn.AddChain(ch)
	if e := d.conn.Flush(); e != nil {
		return nil, nil, fmt.Errorf("add chain filter/FORWARD: %w", e)
	}
	return t, ch, nil
}

func (d *Driver) getChain(fam nft.TableFamily, tableName, chainName string) (*nft.Table, *nft.Chain, error) {
	tables, err := d.conn.ListTables()
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
		// keep going; we'll create it later if needed
	} else {
		chains, err := d.conn.ListChains()
		if err != nil {
			return nil, nil, fmt.Errorf("list chains: %w", err)
		}
		for _, ch := range chains {
			if ch.Table != nil && ch.Table.Family == fam && ch.Table.Name == tableName && ch.Name == chainName {
				return tbl, ch, nil
			}
		}
	}
	return tbl, nil, os.ErrNotExist
}

// -------- expressions --------

func zstr(s string) []byte { return append([]byte(s), 0x00) }

// -o dev -j MASQUERADE
func exprMasqOIF(dev string) []expr.Any {
	return []expr.Any{
		&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: zstr(dev)},
		&expr.Masq{},
	}
}

// -i X -o Y -j ACCEPT
func exprAcceptIIFtoOIF(iif, oif string) []expr.Any {
	return []expr.Any{
		&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: zstr(iif)},
		&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: zstr(oif)},
		&expr.Verdict{Kind: expr.VerdictAccept},
	}
}

// -i dev -o tun -m state --state RELATED,ESTABLISHED -j ACCEPT
func exprAcceptEstablished(iif, oif string) []expr.Any {
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

// -------- add/del by tag (append semantics) --------

func (d *Driver) appendIfMissingByTag(t *nft.Table, ch *nft.Chain, e []expr.Any, tag []byte) error {
	rules, err := d.conn.GetRules(t, ch)
	if err != nil {
		return fmt.Errorf("get rules %s/%s: %w", t.Name, ch.Name, err)
	}
	for _, r := range rules {
		if reflect.DeepEqual(r.UserData, tag) {
			return nil
		}
	}
	d.conn.AddRule(&nft.Rule{Table: t, Chain: ch, Exprs: e, UserData: tag}) // append
	return nil
}

func (d *Driver) delIfPresentByTag(t *nft.Table, ch *nft.Chain, tag []byte) error {
	rules, err := d.conn.GetRules(t, ch)
	if err != nil {
		return fmt.Errorf("get rules %s/%s: %w", t.Name, ch.Name, err)
	}
	for _, r := range rules {
		if reflect.DeepEqual(r.UserData, tag) {
			_ = d.conn.DelRule(r)
			break
		}
	}
	return nil
}

// -------- tags --------

func tagMasq4(dev string) []byte { return []byte("tungo:nat4 oif=" + dev) }
func tagMasq6(dev string) []byte { return []byte("tungo:nat6 oif=" + dev) }

func tagV4Fwd(iif, oif string) []byte    { return []byte("tungo:v4 fwd " + iif + "->" + oif) }
func tagV4FwdRet(iif, oif string) []byte { return []byte("tungo:v4 fwdret " + iif + "->" + oif) }
func tagV6Fwd(iif, oif string) []byte    { return []byte("tungo:v6 fwd " + iif + "->" + oif) }
func tagV6FwdRet(iif, oif string) []byte { return []byte("tungo:v6 fwdret " + iif + "->" + oif) }

// -------- small helpers --------

func readSysctl(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func isAFNotSupported(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return errors.Is(err, syscall.EAFNOSUPPORT) || strings.Contains(s, "address family not supported")
}

func isSeqMismatch(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "mismatched sequence in netlink reply")
}
