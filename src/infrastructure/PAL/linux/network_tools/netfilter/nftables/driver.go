package nftables

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	nft "github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
)

const (
	fwdChainName = "IPTABLES-TUNGO-FWD"
)

type Driver struct {
	tags                   Tags
	ruleSigHandler         RuleSigHandler
	errInterpreter         ErrInterpreter
	interfaceNameValidator InterfaceNameValidator
	mu                     sync.Mutex
	conn                   *nft.Conn
	cfg                    Config
	closed                 bool
}

type Config struct {
	// Netlink retry policy.
	MaxNetlinkRetries int           // default 3
	RetryBackoff      time.Duration // default 80ms

	// Priorities for base chains when we have to create them (iptables-nft compatible).
	NatPostroutingPrio int // default 100
	FilterForwardPrio  int // default 0

	AllowCreateForwardBase     bool // default true
	SetForwardBasePolicyAccept bool // default false
}

func DefaultConfig() Config {
	return Config{
		MaxNetlinkRetries:          3,
		RetryBackoff:               80 * time.Millisecond,
		NatPostroutingPrio:         100,
		FilterForwardPrio:          0,
		AllowCreateForwardBase:     true,
		SetForwardBasePolicyAccept: false,
	}
}

func New() (*Driver, error) {
	d, err := NewWithConfig(DefaultConfig())
	if err != nil {
		return nil, err
	}
	d.tags = NewDefaultTags()
	d.ruleSigHandler = NewDefaultRuleSigHandler()
	d.errInterpreter = NewDefaultErrInterpreter()
	d.interfaceNameValidator = NewDefaultInterfaceNameValidator()
	return d, nil
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
	d.closed = true
	if d.conn != nil {
		err := d.conn.CloseLasting()
		d.conn = nil
		return err
	}
	return nil
}

// ip nat POSTROUTING -o <dev> -j MASQUERADE (append)
// IPv6 — best-effort: errors "not supported" are ignored.
func (d *Driver) EnableDevMasquerade(devName string) error {
	if err := d.interfaceNameValidator.ValidateIfName(devName); err != nil {
		return err
	}
	return d.withRetry(func() error {
		if t4, ch4, err := d.ensureSystemNatPostrouting(nft.TableFamilyIPv4); err != nil {
			return err
		} else {
			if ok, err := d.appendIfMissingByTagOrSig(
				t4, ch4,
				d.ruleSigHandler.sigMasq(devName),
				d.exprMasqOIF(devName),
				d.tags.tagMasq4(devName),
			); err != nil {
				return err
			} else if ok {
				if err := d.conn.Flush(); err != nil {
					return err
				}
			}
		}

		if t6, ch6, err := d.ensureSystemNatPostrouting(nft.TableFamilyIPv6); err == nil {
			if ok, err := d.appendIfMissingByTagOrSig(
				t6, ch6,
				d.ruleSigHandler.sigMasq(devName),
				d.exprMasqOIF(devName),
				d.tags.tagMasq6(devName),
			); err != nil {
				return err
			} else if ok {
				if err := d.conn.Flush(); err != nil {
					if !d.errInterpreter.isNatUnsupported(err) {
						return err
					}
				}
			}
		} else if !(d.errInterpreter.isAFNotSupported(err) || d.errInterpreter.isNatUnsupported(err)) {
			return err
		}

		return nil
	})
}

func (d *Driver) DisableDevMasquerade(devName string) error {
	if err := d.interfaceNameValidator.ValidateIfName(devName); err != nil {
		return err
	}
	return d.withRetry(func() error {
		var needFlush bool

		// v4
		if t, ch, err := d.getSystemNatPostroutingIfExists(nft.TableFamilyIPv4); err == nil && ch != nil {
			ok, err := d.delByTag(t, ch, d.tags.tagMasq4(devName))
			if err != nil {
				return err
			}
			needFlush = needFlush || ok
		} else if err != nil {
			return err
		}

		// v6: best-effort
		if t6, ch6, err := d.getSystemNatPostroutingIfExists(nft.TableFamilyIPv6); err == nil && ch6 != nil {
			ok, err := d.delByTag(t6, ch6, d.tags.tagMasq6(devName))
			if err != nil {
				return err
			}
			needFlush = needFlush || ok
		} else if !(d.errInterpreter.isAFNotSupported(err) || d.errInterpreter.isNatUnsupported(err)) {
			return err
		}

		if needFlush {
			if err := d.conn.Flush(); err != nil && !d.errInterpreter.isNatUnsupported(err) {
				return err
			}
		}
		return nil
	})
}

// ip filter FORWARD (APPEND jump) -> user chain;
//
//	-i <tun> -o <dev> -j ACCEPT (append)
func (d *Driver) EnableForwardingFromTunToDev(tunName, devName string) error {
	if err := d.interfaceNameValidator.ValidateIfName(tunName); err != nil {
		return err
	}
	if err := d.interfaceNameValidator.ValidateIfName(devName); err != nil {
		return err
	}
	return d.withRetry(func() error {
		var needFlush bool

		// v4
		{
			t, chFwd, chUser, err := d.ensureFilterUserChain(nft.TableFamilyIPv4, fwdChainName)
			if err != nil {
				return err
			}
			if ok, err := d.ensureJumpAppend(t, chFwd, chUser.Name); err != nil {
				return err
			} else {
				needFlush = needFlush || ok
			}
			ok, err := d.appendIfMissingByTagOrSig(t, chUser, d.ruleSigHandler.sigFwd(tunName, devName, false), d.exprAcceptIIFtoOIF(tunName, devName), d.tags.tagV4Fwd(tunName, devName))
			if err != nil {
				return err
			}
			needFlush = needFlush || ok
		}

		// v6 (best-effort)
		{
			t6, chFwd6, chUser6, err := d.ensureFilterUserChain(nft.TableFamilyIPv6, fwdChainName)
			if err != nil {
				if d.errInterpreter.isAFNotSupported(err) {
				} else {
					return err
				}
			} else {
				if ok, err := d.ensureJumpAppend(t6, chFwd6, chUser6.Name); err != nil {
					return err
				} else {
					needFlush = needFlush || ok
				}
				ok, err := d.appendIfMissingByTagOrSig(t6, chUser6, d.ruleSigHandler.sigFwd(tunName, devName, false),
					d.exprAcceptIIFtoOIF(tunName, devName), d.tags.tagV6Fwd(tunName, devName))
				if err != nil {
					return err
				}
				needFlush = needFlush || ok
			}
		}

		if needFlush {
			if err := d.conn.Flush(); err != nil {
				return err
			}
		}
		return nil
	})
}

func (d *Driver) DisableForwardingFromTunToDev(tunName, devName string) error {
	if err := d.interfaceNameValidator.ValidateIfName(tunName); err != nil {
		return err
	}
	if err := d.interfaceNameValidator.ValidateIfName(devName); err != nil {
		return err
	}
	return d.withRetry(func() error {
		var needFlush bool

		// v4
		{
			if t, chFwd, chUser, err := d.getFilterUserChainIfExists(nft.TableFamilyIPv4, fwdChainName); err == nil && chUser != nil {
				ok, err := d.delByTag(t, chUser, d.tags.tagV4Fwd(tunName, devName))
				if err != nil {
					return err
				}
				needFlush = needFlush || ok
				ok2, err := d.cleanupUserChainIfEmpty(t, chFwd, chUser)
				if err != nil {
					return err
				}
				needFlush = needFlush || ok2
			} else if err != nil {
				return err
			}
		}

		// v6
		{
			if t6, chFwd6, chUser6, err := d.getFilterUserChainIfExists(nft.TableFamilyIPv6, fwdChainName); err == nil && chUser6 != nil {
				ok, err := d.delByTag(t6, chUser6, d.tags.tagV6Fwd(tunName, devName))
				if err != nil {
					return err
				}
				needFlush = needFlush || ok
				ok2, err := d.cleanupUserChainIfEmpty(t6, chFwd6, chUser6)
				if err != nil {
					return err
				}
				needFlush = needFlush || ok2
			} else if err != nil && !d.errInterpreter.isAFNotSupported(err) {
				return err
			}
		}

		if needFlush {
			if err := d.conn.Flush(); err != nil {
				return err
			}
		}
		return nil
	})
}

// ip filter FORWARD jump->user chain;
//
//	-i <dev> -o <tun> -m state --state RELATED,ESTABLISHED -j ACCEPT
func (d *Driver) EnableForwardingFromDevToTun(tunName, devName string) error {
	if err := d.interfaceNameValidator.ValidateIfName(tunName); err != nil {
		return err
	}
	if err := d.interfaceNameValidator.ValidateIfName(devName); err != nil {
		return err
	}
	return d.withRetry(func() error {
		var needFlush bool

		// v4
		{
			t, chFwd, chUser, err := d.ensureFilterUserChain(nft.TableFamilyIPv4, fwdChainName)
			if err != nil {
				return err
			}
			if ok, err := d.ensureJumpAppend(t, chFwd, chUser.Name); err != nil {
				return err
			} else {
				needFlush = needFlush || ok
			}
			ok, err := d.appendIfMissingByTagOrSig(t, chUser, d.ruleSigHandler.sigFwd(devName, tunName, true), d.exprAcceptEstablished(devName, tunName), d.tags.tagV4FwdRet(devName, tunName))
			if err != nil {
				return err
			}
			needFlush = needFlush || ok
		}

		// v6
		{
			t6, chFwd6, chUser6, err := d.ensureFilterUserChain(nft.TableFamilyIPv6, fwdChainName)
			if err != nil {
				if d.errInterpreter.isAFNotSupported(err) {
				} else {
					return err
				}
			} else {
				if ok, err := d.ensureJumpAppend(t6, chFwd6, chUser6.Name); err != nil {
					return err
				} else {
					needFlush = needFlush || ok
				}
				ok, err := d.appendIfMissingByTagOrSig(
					t6, chUser6,
					d.ruleSigHandler.sigFwd(devName, tunName, true),
					d.exprAcceptEstablished(devName, tunName),
					d.tags.tagV6FwdRet(devName, tunName),
				)
				if err != nil {
					return err
				}
				needFlush = needFlush || ok
			}
		}

		if needFlush {
			if err := d.conn.Flush(); err != nil {
				return err
			}
		}
		return nil
	})
}

func (d *Driver) DisableForwardingFromDevToTun(tunName, devName string) error {
	if err := d.interfaceNameValidator.ValidateIfName(tunName); err != nil {
		return err
	}
	if err := d.interfaceNameValidator.ValidateIfName(devName); err != nil {
		return err
	}
	return d.withRetry(func() error {
		var needFlush bool

		// v4
		{
			if t, chFwd, chUser, err := d.getFilterUserChainIfExists(nft.TableFamilyIPv4, fwdChainName); err == nil && chUser != nil {
				ok, err := d.delByTag(t, chUser, d.tags.tagV4FwdRet(devName, tunName))
				if err != nil {
					return err
				}
				needFlush = needFlush || ok
				ok2, err := d.cleanupUserChainIfEmpty(t, chFwd, chUser)
				if err != nil {
					return err
				}
				needFlush = needFlush || ok2
			} else if err != nil {
				return err
			}
		}

		// v6
		{
			if t6, chFwd6, chUser6, err := d.getFilterUserChainIfExists(nft.TableFamilyIPv6, fwdChainName); err == nil && chUser6 != nil {
				ok, err := d.delByTag(t6, chUser6, d.tags.tagV6FwdRet(devName, tunName))
				if err != nil {
					return err
				}
				needFlush = needFlush || ok
				ok2, err := d.cleanupUserChainIfEmpty(t6, chFwd6, chUser6)
				if err != nil {
					return err
				}
				needFlush = needFlush || ok2
			} else if err != nil && !d.errInterpreter.isAFNotSupported(err) {
				return err
			}
		}

		if needFlush {
			if err := d.conn.Flush(); err != nil {
				return err
			}
		}
		return nil
	})
}

func (d *Driver) ConfigureMssClamping(_ string) error {
	return errors.New("MSS clamping via nft is not implemented; prefer sysctl net.ipv4.tcp_mtu_probing=1")
}

// -------------------- internals --------------------
func (d *Driver) withRetry(op func() error) error {
	var last error
	maxNetlinkRetries := d.cfg.MaxNetlinkRetries
	if maxNetlinkRetries <= 0 {
		maxNetlinkRetries = 1
	}
	for i := 0; i < maxNetlinkRetries; i++ {
		if i > 0 && d.cfg.RetryBackoff > 0 {
			base := d.cfg.RetryBackoff
			j := time.Duration(rand.Int63n(int64(base)))
			time.Sleep(base + j)
		}
		d.mu.Lock()
		if d.closed {
			d.mu.Unlock()
			return errors.New("nft driver is closed")
		}
		if i > 0 || d.conn == nil {
			_ = d.resetConnLocked()
		}
		err := op()
		d.mu.Unlock()

		if err == nil {
			return nil
		}
		last = err
		if d.errInterpreter.isSeqMismatch(err) || d.errInterpreter.isTransientNetlink(err) {
			continue
		}
		return err
	}
	return last
}

func (d *Driver) resetConnLocked() error {
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
	t, ch, err := d.getChain(fam, "nat", "POSTROUTING")
	if err == nil && chainIsBase(ch, nft.ChainTypeNAT, *nft.ChainHookPostrouting) {
		return t, ch, nil
	}
	if d.errInterpreter.isAFNotSupported(err) {
		return nil, nil, err
	}
	if err == nil && ch != nil {
		return nil, nil, fmt.Errorf("nat/POSTROUTING exists but is not a base NAT chain")
	}

	// create table if missing
	createdTable := false
	if t == nil {
		t = &nft.Table{Family: fam, Name: "nat"}
		d.conn.AddTable(t)
		if e := d.conn.Flush(); e != nil {
			if d.errInterpreter.isAlreadyExists(e) {
				if tt, cc, ge := d.getChain(fam, "nat", "POSTROUTING"); ge == nil && cc != nil {
					return tt, cc, nil
				}
			}
			if d.errInterpreter.isNatUnsupported(e) {
				return nil, nil, e
			}
			return nil, nil, fmt.Errorf("add table %v/nat: %w", fam, e)
		}
		createdTable = true
	}

	// create base chain POSTROUTING
	h := *nft.ChainHookPostrouting
	p := nft.ChainPriority(d.cfg.NatPostroutingPrio)
	ch = &nft.Chain{Table: t, Name: "POSTROUTING", Type: nft.ChainTypeNAT, Hooknum: &h, Priority: &p}
	d.conn.AddChain(ch)
	if e := d.conn.Flush(); e != nil {
		if createdTable {
			d.conn.DelTable(t)
			_ = d.conn.Flush()
		}
		if d.errInterpreter.isAlreadyExists(e) {
			if tt, cc, ge := d.getChain(fam, "nat", "POSTROUTING"); ge == nil && cc != nil {
				return tt, cc, nil
			}
		}
		if d.errInterpreter.isNatUnsupported(e) {
			return nil, nil, e
		}
		return nil, nil, fmt.Errorf("add chain nat/POSTROUTING: %w", e)
	}
	return t, ch, nil
}

func chainIsBase(ch *nft.Chain, wantType nft.ChainType, wantHook nft.ChainHook) bool {
	return ch != nil && ch.Type == wantType && ch.Hooknum != nil && *ch.Hooknum == wantHook
}

// --- ensure filter user chain + jump from system FORWARD (append semantics) ---

func (d *Driver) ensureFilterUserChain(fam nft.TableFamily, childName string) (tbl *nft.Table, fwd *nft.Chain, child *nft.Chain, err error) {
	// ensure filter table
	tbl, ch, err := d.getChain(fam, "filter", "FORWARD")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		if d.errInterpreter.isAFNotSupported(err) {
			return nil, nil, nil, err
		}
		return nil, nil, nil, err
	}
	if err == nil && ch != nil && !chainIsBase(ch, nft.ChainTypeFilter, *nft.ChainHookForward) {
		return nil, nil, nil, fmt.Errorf("filter/FORWARD exists but is not a base filter chain")
	}
	if tbl == nil {
		tbl = &nft.Table{Family: fam, Name: "filter"}
		d.conn.AddTable(tbl)
		if e := d.conn.Flush(); e != nil {
			if d.errInterpreter.isAFNotSupported(e) {
				return nil, nil, nil, e
			}
			if d.errInterpreter.isAlreadyExists(e) {
				if tbl == nil {
					tbl = &nft.Table{Family: fam, Name: "filter"}
				}
			} else {
				return nil, nil, nil, fmt.Errorf("add table %v/filter: %w", fam, e)
			}
		}
	}

	_, fwd, err = d.getChain(fam, "filter", "FORWARD")
	if fwd == nil {
		if !d.cfg.AllowCreateForwardBase {
			return nil, nil, nil, fmt.Errorf("filter/FORWARD base chain missing and creation disabled")
		}
		h := *nft.ChainHookForward
		p := nft.ChainPriority(d.cfg.FilterForwardPrio)
		fwd = &nft.Chain{Table: tbl, Name: "FORWARD", Type: nft.ChainTypeFilter, Hooknum: &h, Priority: &p}
		if d.cfg.SetForwardBasePolicyAccept {
			pol := nft.ChainPolicyAccept
			fwd.Policy = &pol
		}
		d.conn.AddChain(fwd)
		if e := d.conn.Flush(); e != nil {
			if d.errInterpreter.isAFNotSupported(e) {
				return nil, nil, nil, e
			}
			if d.errInterpreter.isAlreadyExists(e) {
				if _, ff, ge := d.getChain(fam, "filter", "FORWARD"); ge == nil && ff != nil {
					fwd = ff
				} else {
					return nil, nil, nil, fmt.Errorf("recover chain filter/FORWARD: %w", ge)
				}
			} else {
				return nil, nil, nil, fmt.Errorf("add chain filter/FORWARD: %w", e)
			}
		}
	}

	// ensure child user chain
	_, child, err = d.getChain(fam, "filter", childName)
	if child == nil {
		child = &nft.Chain{Table: tbl, Name: childName}
		d.conn.AddChain(child)
		if e := d.conn.Flush(); e != nil {
			if d.errInterpreter.isAFNotSupported(e) {
				return nil, nil, nil, e
			}
			if d.errInterpreter.isAlreadyExists(e) {
				if _, cc, ge := d.getChain(fam, "filter", childName); ge == nil && cc != nil {
					child = cc
				} else {
					return nil, nil, nil, fmt.Errorf("recover chain filter/%s: %w", childName, ge)
				}
			} else {
				return nil, nil, nil, fmt.Errorf("add chain filter/%s: %w", childName, e)
			}
		}
	}
	return tbl, fwd, child, nil
}

func (d *Driver) ensureJumpAppend(t *nft.Table, chFwd *nft.Chain, childName string) (bool, error) {
	tag := d.tags.tagHookJump(childName)
	want := d.ruleSigHandler.sigJump(childName)

	rs, err := d.conn.GetRules(t, chFwd)
	if err != nil {
		return false, fmt.Errorf("get rules %s/%s: %w", t.Name, chFwd.Name, err)
	}

	lastMatchIdx := -1
	lastOurIdx := -1
	var ourIdxs []int

	for i, r := range rs {
		isOur := d.tags.hasTag(r, tag)

		isMatch := isOur
		if !isMatch {
			if sig, ok := d.ruleSigHandler.sigFromExprs(r.Exprs); ok && d.ruleSigHandler.sigEqual(sig, want) {
				isMatch = true
			}
		}

		if isOur {
			ourIdxs = append(ourIdxs, i)
			lastOurIdx = i
		}
		if isMatch {
			lastMatchIdx = i
		}
	}

	changed := false

	switch {
	case lastMatchIdx >= 0 && lastMatchIdx == lastOurIdx:
		for _, idx := range ourIdxs {
			if idx != lastOurIdx {
				_ = d.conn.DelRule(rs[idx])
				changed = true
			}
		}
		return changed, nil

	case lastMatchIdx >= 0 && lastMatchIdx != lastOurIdx:
		for _, idx := range ourIdxs {
			_ = d.conn.DelRule(rs[idx])
			changed = true
		}
		return changed, nil

	default:
		for _, idx := range ourIdxs {
			_ = d.conn.DelRule(rs[idx])
			changed = true
		}
		d.conn.AddRule(&nft.Rule{
			Table:    t,
			Chain:    chFwd,
			Exprs:    d.exprJumpTo(childName),
			UserData: tag,
		})
		return true, nil
	}
}

func (d *Driver) appendIfMissingByTagOrSig(t *nft.Table, ch *nft.Chain, want RuleSig, e []expr.Any, tag []byte) (changed bool, err error) {
	rules, err := d.conn.GetRules(t, ch)
	if err != nil {
		return false, fmt.Errorf("get rules %s/%s: %w", t.Name, ch.Name, err)
	}
	for _, r := range rules {
		if d.tags.hasTag(r, tag) {
			return false, nil
		}
		if sig, ok := d.ruleSigHandler.sigFromExprs(r.Exprs); ok && d.ruleSigHandler.sigEqual(sig, want) {
			return false, nil
		}
	}
	d.conn.AddRule(&nft.Rule{Table: t, Chain: ch, Exprs: e, UserData: tag}) // append
	return true, nil
}

func (d *Driver) delByTag(t *nft.Table, ch *nft.Chain, tag []byte) (bool, error) {
	rules, err := d.conn.GetRules(t, ch)
	if err != nil {
		return false, fmt.Errorf("get rules %s/%s: %w", t.Name, ch.Name, err)
	}
	changed := false
	for _, r := range rules {
		if d.tags.hasTag(r, tag) {
			_ = d.conn.DelRule(r)
			changed = true
		}
	}
	return changed, nil
}

func (d *Driver) delJumpIfPresent(t *nft.Table, chFwd *nft.Chain, childName string) (bool, error) {
	tag := d.tags.tagHookJump(childName)
	wantSig := d.ruleSigHandler.sigJump(childName)
	rules, err := d.conn.GetRules(t, chFwd)
	if err != nil {
		return false, fmt.Errorf("get rules %s/%s: %w", t.Name, chFwd.Name, err)
	}
	changed := false
	for _, r := range rules {
		if d.tags.hasTag(r, tag) {
			_ = d.conn.DelRule(r)
			changed = true
			continue
		}
		if sig, ok := d.ruleSigHandler.sigFromExprs(r.Exprs); ok && d.ruleSigHandler.sigEqual(sig, wantSig) {
			_ = d.conn.DelRule(r)
			changed = true
		}
	}
	return changed, nil
}

func (d *Driver) cleanupUserChainIfEmpty(t *nft.Table, chFwd *nft.Chain, child *nft.Chain) (bool, error) {
	if t == nil || chFwd == nil || child == nil {
		return false, nil
	}

	rs, err := d.conn.GetRules(t, child)
	if err != nil {
		ls := strings.ToLower(err.Error())
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOENT) || strings.Contains(ls, "no such file or directory") {
			return false, nil
		}
		return false, fmt.Errorf("get rules %s/%s: %w", t.Name, child.Name, err)
	}

	if len(rs) > 0 {
		return false, nil
	}

	changed := false
	if del, err := d.delJumpIfPresent(t, chFwd, child.Name); err != nil {
		return false, err
	} else if del {
		changed = true
	}

	d.conn.DelChain(child)
	changed = true

	return changed, nil
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
		return nil, nil, os.ErrNotExist
	}
	chains, err := d.conn.ListChains()
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

func (d *Driver) zstr(s string) []byte { return append([]byte(s), 0x00) }

// -o dev -j MASQUERADE
func (d *Driver) exprMasqOIF(dev string) []expr.Any {
	return []expr.Any{
		&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: d.zstr(dev)},
		&expr.Masq{},
	}
}

// -i X -o Y -j ACCEPT
func (d *Driver) exprAcceptIIFtoOIF(iif, oif string) []expr.Any {
	return []expr.Any{
		&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: d.zstr(iif)},
		&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: d.zstr(oif)},
		&expr.Verdict{Kind: expr.VerdictAccept},
	}
}

// -i dev -o tun -m state --state RELATED,ESTABLISHED -j ACCEPT
func (d *Driver) exprAcceptEstablished(iif, oif string) []expr.Any {
	mask := binaryutil.BigEndian.PutUint32(expr.CtStateBitESTABLISHED | expr.CtStateBitRELATED)
	return []expr.Any{
		&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: d.zstr(iif)},
		&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 1},
		&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: d.zstr(oif)},
		&expr.Ct{Register: 1, Key: expr.CtKeySTATE},
		&expr.Bitwise{SourceRegister: 1, DestRegister: 1, Len: 4, Mask: mask, Xor: []byte{0, 0, 0, 0}},
		&expr.Cmp{Op: expr.CmpOpNeq, Register: 1, Data: []byte{0, 0, 0, 0}},
		&expr.Verdict{Kind: expr.VerdictAccept},
	}
}

func (d *Driver) exprJumpTo(chain string) []expr.Any {
	return []expr.Any{
		&expr.Verdict{Kind: expr.VerdictJump, Chain: chain},
	}
}

func (d *Driver) getSystemNatPostroutingIfExists(fam nft.TableFamily) (*nft.Table, *nft.Chain, error) {
	t, ch, err := d.getChain(fam, "nat", "POSTROUTING")
	if err == nil && ch != nil {
		return t, ch, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if d.errInterpreter.isAFNotSupported(err) || d.errInterpreter.isNatUnsupported(err) {
		return nil, nil, nil
	}
	return nil, nil, err
}

func (d *Driver) getFilterUserChainIfExists(fam nft.TableFamily, childName string) (*nft.Table, *nft.Chain, *nft.Chain, error) {
	tbl, fwd, err := d.getChain(fam, "filter", "FORWARD")
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil, nil
	}
	if err != nil {
		if d.errInterpreter.isAFNotSupported(err) {
			return nil, nil, nil, err
		}
		return nil, nil, nil, err
	}
	_, child, err := d.getChain(fam, "filter", childName)
	if errors.Is(err, os.ErrNotExist) {
		return tbl, fwd, nil, nil
	}
	if err != nil {
		return nil, nil, nil, err
	}
	return tbl, fwd, child, nil
}
