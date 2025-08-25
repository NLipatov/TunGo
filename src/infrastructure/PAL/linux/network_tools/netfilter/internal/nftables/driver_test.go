package nftables

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"syscall"
	"testing"

	nft "github.com/google/nftables"
	"github.com/google/nftables/expr"
)

// ---------------------- in-memory fake for conn ----------------------

type fakeConn struct {
	tables []*nft.Table
	chains []*nft.Chain
	rules  map[*nft.Chain][]*nft.Rule

	// error/behavior injection
	failIPv6OnAddTable bool  // simulate AF_INET6 unsupported: next Flush() fails with EAFNOSUPPORT
	nextFlushErr       error // next Flush() returns this error and then resets

	listTablesErr error // returned once, then cleared
	listChainsErr error // returned once, then cleared
}

func (f *fakeConn) ListTables() ([]*nft.Table, error) {
	if f.listTablesErr != nil {
		e := f.listTablesErr
		f.listTablesErr = nil
		return nil, e
	}
	out := make([]*nft.Table, len(f.tables))
	copy(out, f.tables)
	return out, nil
}
func (f *fakeConn) ListChains() ([]*nft.Chain, error) {
	if f.listChainsErr != nil {
		e := f.listChainsErr
		f.listChainsErr = nil
		return nil, e
	}
	out := make([]*nft.Chain, len(f.chains))
	copy(out, f.chains)
	return out, nil
}
func (f *fakeConn) AddTable(t *nft.Table) *nft.Table {
	f.tables = append(f.tables, t)
	if t.Family == nft.TableFamilyIPv6 && f.failIPv6OnAddTable {
		f.nextFlushErr = syscall.EAFNOSUPPORT
	}
	return t
}
func (f *fakeConn) AddChain(ch *nft.Chain) *nft.Chain {
	f.chains = append(f.chains, ch)
	return ch
}
func (f *fakeConn) GetRules(_ *nft.Table, ch *nft.Chain) ([]*nft.Rule, error) {
	rs := f.rules[ch]
	out := make([]*nft.Rule, len(rs))
	copy(out, rs)
	return out, nil
}
func (f *fakeConn) AddRule(r *nft.Rule) *nft.Rule {
	if f.rules == nil {
		f.rules = map[*nft.Chain][]*nft.Rule{}
	}
	f.rules[r.Chain] = append(f.rules[r.Chain], r)
	return r
}
func (f *fakeConn) DelRule(r *nft.Rule) error {
	rs := f.rules[r.Chain]
	for i, rr := range rs {
		if rr == r || reflect.DeepEqual(rr.UserData, r.UserData) {
			f.rules[r.Chain] = append(rs[:i], rs[i+1:]...)
			return nil
		}
	}
	return nil
}
func (f *fakeConn) Flush() error {
	e := f.nextFlushErr
	f.nextFlushErr = nil
	return e
}
func (f *fakeConn) CloseLasting() error { return nil }

// helpers
func findChain(t *testing.T, f *fakeConn, fam nft.TableFamily, table, chain string) *nft.Chain {
	t.Helper()
	for _, ch := range f.chains {
		if ch.Table != nil && ch.Table.Family == fam && ch.Table.Name == table && ch.Name == chain {
			return ch
		}
	}
	t.Fatalf("chain %s/%s (fam=%v) not found", table, chain, fam)
	return nil
}
func hasRuleWithTag(f *fakeConn, ch *nft.Chain, tag string) bool {
	for _, r := range f.rules[ch] {
		if string(r.UserData) == tag {
			return true
		}
	}
	return false
}
func rulesCount(f *fakeConn, ch *nft.Chain) int { return len(f.rules[ch]) }

// ------------------------------ tests --------------------------------

func TestEnableDisableMasquerade_V4Only_SkipsIPv6Unsupported(t *testing.T) {
	fc := &fakeConn{failIPv6OnAddTable: true}
	cfg := DefaultConfig()
	b, _ := NewBackendWithConfigAndConn(fc, cfg)

	if err := b.EnableDevMasquerade("eth0"); err != nil {
		t.Fatalf("EnableDevMasquerade error: %v", err)
	}
	// v4 postrouting exists with tag
	ch4 := findChain(t, fc, nft.TableFamilyIPv4, cfg.TableNat4Name, cfg.PostroutingChainName)
	if !hasRuleWithTag(fc, ch4, "tungo:masq4 oif=eth0") {
		t.Fatalf("missing v4 masq rule")
	}
	// no v6 artifacts when AF not supported
	for _, ch := range fc.chains {
		if ch.Table != nil && ch.Table.Family == nft.TableFamilyIPv6 && ch.Table.Name == cfg.TableNat6Name {
			t.Fatalf("unexpected IPv6 objects when AF_INET6 unsupported")
		}
	}
	// disable removes v4 rule
	if err := b.DisableDevMasquerade("eth0"); err != nil {
		t.Fatalf("DisableDevMasquerade error: %v", err)
	}
	if hasRuleWithTag(fc, ch4, "tungo:masq4 oif=eth0") {
		t.Fatalf("v4 masq rule still present after disable")
	}
}

func TestEnableDisableMasquerade_V4V6_Success(t *testing.T) {
	fc := &fakeConn{} // IPv6 supported
	cfg := DefaultConfig()
	b, _ := NewBackendWithConfigAndConn(fc, cfg)

	if err := b.EnableDevMasquerade("eth0"); err != nil {
		t.Fatalf("EnableDevMasquerade error: %v", err)
	}
	ch4 := findChain(t, fc, nft.TableFamilyIPv4, cfg.TableNat4Name, cfg.PostroutingChainName)
	ch6 := findChain(t, fc, nft.TableFamilyIPv6, cfg.TableNat6Name, cfg.PostroutingChainName)
	if !hasRuleWithTag(fc, ch4, "tungo:masq4 oif=eth0") || !hasRuleWithTag(fc, ch6, "tungo:masq6 oif=eth0") {
		t.Fatalf("missing v4 or v6 masq rule")
	}
	if err := b.DisableDevMasquerade("eth0"); err != nil {
		t.Fatalf("DisableDevMasquerade error: %v", err)
	}
	if hasRuleWithTag(fc, ch4, "tungo:masq4 oif=eth0") || hasRuleWithTag(fc, ch6, "tungo:masq6 oif=eth0") {
		t.Fatalf("masq rules not removed")
	}
}

func TestForward_DockerUser_AddRemove_Idempotent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PreferDockerUser = true
	fc := &fakeConn{}

	// Prepare Docker DOCKER-USER chains in ip and ip6
	tbl4 := &nft.Table{Family: nft.TableFamilyIPv4, Name: "filter"}
	tbl6 := &nft.Table{Family: nft.TableFamilyIPv6, Name: "filter"}
	chUsr4 := &nft.Chain{Table: tbl4, Name: "DOCKER-USER"}
	chUsr6 := &nft.Chain{Table: tbl6, Name: "DOCKER-USER"}
	fc.AddTable(tbl4)
	fc.AddTable(tbl6)
	fc.AddChain(chUsr4)
	fc.AddChain(chUsr6)

	b, _ := NewBackendWithConfigAndConn(fc, cfg)

	// first apply
	if err := b.EnableForwardingFromTunToDev("tun0", "eth0"); err != nil {
		t.Fatalf("EnableForwardingFromTunToDev error: %v", err)
	}
	// idempotent apply
	if err := b.EnableForwardingFromTunToDev("tun0", "eth0"); err != nil {
		t.Fatalf("EnableForwardingFromTunToDev (2nd) error: %v", err)
	}

	if !hasRuleWithTag(fc, chUsr4, "tungo:docker fwd ip iif=tun0 oif=eth0") ||
		!hasRuleWithTag(fc, chUsr4, "tungo:docker fwdret ip iif=eth0 oif=tun0") ||
		!hasRuleWithTag(fc, chUsr6, "tungo:docker fwd ip6 iif=tun0 oif=eth0") ||
		!hasRuleWithTag(fc, chUsr6, "tungo:docker fwdret ip6 iif=eth0 oif=tun0") {
		t.Fatalf("docker-user forward rules missing")
	}

	// ensure no duplicates
	if rulesCount(fc, chUsr4) != 2 || rulesCount(fc, chUsr6) != 2 {
		t.Fatalf("expected exactly 2 rules per DOCKER-USER (fwd,fwdret), got ip=%d ip6=%d", rulesCount(fc, chUsr4), rulesCount(fc, chUsr6))
	}

	// remove
	if err := b.DisableForwardingFromTunToDev("tun0", "eth0"); err != nil {
		t.Fatalf("DisableForwardingFromTunToDev error: %v", err)
	}
	if rulesCount(fc, chUsr4) != 0 || rulesCount(fc, chUsr6) != 0 {
		t.Fatalf("docker-user rules not fully removed")
	}
}

func TestForward_FallbackInet_AddRemove_NoDockerUser(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PreferDockerUser = true // will try, not found, fallback to inet
	fc := &fakeConn{}
	b, _ := NewBackendWithConfigAndConn(fc, cfg)

	if err := b.EnableForwardingFromTunToDev("wstun0", "enp1s0"); err != nil {
		t.Fatalf("EnableForwardingFromTunToDev error: %v", err)
	}
	ch := findChain(t, fc, nft.TableFamilyINet, cfg.TableInetName, cfg.ForwardChainName)
	if !hasRuleWithTag(fc, ch, "tungo:fwd iif=wstun0 oif=enp1s0") ||
		!hasRuleWithTag(fc, ch, "tungo:fwdret iif=enp1s0 oif=wstun0") {
		t.Fatalf("missing inet fallback forward rules")
	}
	// idempotent
	if err := b.EnableForwardingFromTunToDev("wstun0", "enp1s0"); err != nil {
		t.Fatalf("EnableForwardingFromTunToDev (2nd) error: %v", err)
	}
	if rulesCount(fc, ch) != 2 {
		t.Fatalf("expected exactly 2 rules (fwd,fwdret) in inet forward, got %d", rulesCount(fc, ch))
	}
	// remove
	if err := b.DisableForwardingFromTunToDev("wstun0", "enp1s0"); err != nil {
		t.Fatalf("DisableForwardingFromTunToDev error: %v", err)
	}
	if rulesCount(fc, ch) != 0 {
		t.Fatalf("inet fallback rules not removed")
	}
}

func TestForward_DockerUser_ConntrackMissingAnnotated(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PreferDockerUser = true
	fc := &fakeConn{}
	// set up only IPv4 DOCKER-USER
	tbl4 := &nft.Table{Family: nft.TableFamilyIPv4, Name: "filter"}
	chUsr4 := &nft.Chain{Table: tbl4, Name: "DOCKER-USER"}
	fc.AddTable(tbl4)
	fc.AddChain(chUsr4)
	// simulate missing conntrack
	fc.nextFlushErr = syscall.EOPNOTSUPP

	b, _ := NewBackendWithConfigAndConn(fc, cfg)
	err := b.EnableForwardingFromTunToDev("tun0", "eth0")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "conntrack") {
		t.Fatalf("expected annotated conntrack error, got: %v", err)
	}
}

func TestEnsureTable_ListTablesErrorPropagates(t *testing.T) {
	fc := &fakeConn{listTablesErr: errors.New("boom")}
	b, _ := NewBackendWithConfigAndConn(fc, DefaultConfig())

	err := b.EnableDevMasquerade("eth0")
	if err == nil || !strings.Contains(err.Error(), "list tables") {
		t.Fatalf("expected 'list tables' error, got: %v", err)
	}
}

func TestEnsureBaseChain_ListChainsErrorPropagates(t *testing.T) {
	fc := &fakeConn{listChainsErr: errors.New("whoops")}
	b, _ := NewBackendWithConfigAndConn(fc, DefaultConfig())

	err := b.EnableDevMasquerade("eth0")
	if err == nil || !strings.Contains(err.Error(), "list chains") {
		t.Fatalf("expected 'list chains' error, got: %v", err)
	}
}

func TestHelpers_readSysctl_isAFNotSupported_isConntrackMissing(t *testing.T) {
	// readSysctl on a temp file
	tmp, err := os.CreateTemp(t.TempDir(), "ipf")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func(tmp *os.File) {
		_ = tmp.Close()
	}(tmp)
	if _, err := tmp.WriteString(" 1 \n"); err != nil {
		t.Fatalf("write tmp: %v", err)
	}
	got, err := readSysctl(tmp.Name())
	if err != nil || got != "1" {
		t.Fatalf("readSysctl got %q, err=%v; want 1,nil", got, err)
	}

	// AF not supported detector
	if !isAFNotSupported(syscall.EAFNOSUPPORT) {
		t.Fatalf("isAFNotSupported(EAFNOSUPPORT)=false, want true")
	}
	if isAFNotSupported(errors.New("other")) {
		t.Fatalf("isAFNotSupported(other)=true, want false")
	}

	// conntrack missing detector
	if !isConntrackMissing(syscall.EOPNOTSUPP) {
		t.Fatalf("isConntrackMissing(EOPNOTSUPP)=false, want true")
	}
	if !isConntrackMissing(errors.New("foo conntrack bar")) {
		t.Fatalf("isConntrackMissing('conntrack' text)=false, want true")
	}
	if isConntrackMissing(errors.New("nope")) {
		t.Fatalf("isConntrackMissing(other)=true, want false")
	}
}

func TestClose_NoPanic(t *testing.T) {
	fc := &fakeConn{}
	b, _ := NewBackendWithConfigAndConn(fc, DefaultConfig())
	if err := b.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
}

// Sanity: expression builders compile and are sensible.
func TestExprBuilders_Sanity(t *testing.T) {
	rs := exprMasqueradeForOIF("eth0")
	if len(rs) == 0 {
		t.Fatalf("exprMasqueradeForOIF returned empty")
	}
	rs2 := exprForwardAcceptIIFtoOIF("i", "o")
	rs3 := exprForwardAcceptEstablished("i", "o")
	_ = []expr.Any{rs2[0], rs3[0]} // just ensure non-empty and types OK
}
func TestEnableDevMasquerade_EmptyName(t *testing.T) {
	fc := &fakeConn{}
	b, _ := NewBackendWithConfigAndConn(fc, DefaultConfig())
	if err := b.EnableDevMasquerade(""); err == nil {
		t.Fatalf("want error on empty dev name")
	}
}

func TestDisableDevMasquerade_EmptyName(t *testing.T) {
	fc := &fakeConn{}
	b, _ := NewBackendWithConfigAndConn(fc, DefaultConfig())
	if err := b.DisableDevMasquerade(""); err == nil {
		t.Fatalf("want error on empty dev name")
	}
}

func TestForward_EmptyNames(t *testing.T) {
	fc := &fakeConn{}
	b, _ := NewBackendWithConfigAndConn(fc, DefaultConfig())
	if err := b.EnableForwardingFromTunToDev("", "eth0"); err == nil {
		t.Fatalf("want error on empty iface")
	}
	if err := b.EnableForwardingFromTunToDev("tun0", ""); err == nil {
		t.Fatalf("want error on empty iface")
	}
}

func TestClose_NilReceiver(t *testing.T) {
	// Calling method on a nil receiver should be a no-op.
	var b *Nftables
	if err := b.Close(); err != nil {
		t.Fatalf("nil receiver Close should not error: %v", err)
	}
}

func TestEnableDevMasquerade_FlushError_AddTableV4(t *testing.T) {
	fc := &fakeConn{nextFlushErr: errors.New("add-table-fail")}
	b, _ := NewBackendWithConfigAndConn(fc, DefaultConfig())

	err := b.EnableDevMasquerade("eth0")
	if err == nil {
		t.Fatalf("expected add-table error")
	}
	s := err.Error()
	// family may be printed as "2" or "ip"
	if !strings.Contains(s, "add table ") ||
		!strings.Contains(s, "/tungo_nat: add-table-fail") {
		t.Fatalf("expected add-table v4 error, got: %v", err)
	}
}

func TestEnableDevMasquerade_FlushError_AddChainV4(t *testing.T) {
	// Pre-create v4 table to force chain creation flush to fail.
	cfg := DefaultConfig()
	fc := &fakeConn{}
	tbl4 := &nft.Table{Family: nft.TableFamilyIPv4, Name: cfg.TableNat4Name}
	fc.AddTable(tbl4)
	fc.nextFlushErr = errors.New("add-chain-fail")

	b, _ := NewBackendWithConfigAndConn(fc, cfg)
	err := b.EnableDevMasquerade("eth0")
	if err == nil || !strings.Contains(err.Error(), "add chain tungo_nat/postrouting: add-chain-fail") {
		t.Fatalf("expected add-chain error, got %v", err)
	}
}

func TestEnableDevMasquerade_FinalFlushError(t *testing.T) {
	cfg := DefaultConfig()
	fc := &fakeConn{}

	// Pre-create BOTH v4 and v6 NAT tables and postrouting chains,
	// so ensureTableFlushed/ensureBaseChainFlushed do NOT call Flush().
	tbl4 := &nft.Table{Family: nft.TableFamilyIPv4, Name: cfg.TableNat4Name}
	ch4 := &nft.Chain{Table: tbl4, Name: cfg.PostroutingChainName}
	fc.AddTable(tbl4)
	fc.AddChain(ch4)

	tbl6 := &nft.Table{Family: nft.TableFamilyIPv6, Name: cfg.TableNat6Name}
	ch6 := &nft.Chain{Table: tbl6, Name: cfg.PostroutingChainName}
	fc.AddTable(tbl6)
	fc.AddChain(ch6)

	// Now make the ONLY remaining Flush (the final one) fail.
	fc.nextFlushErr = errors.New("final-flush-fail")

	b, _ := NewBackendWithConfigAndConn(fc, cfg)
	err := b.EnableDevMasquerade("eth0")
	if err == nil || !strings.Contains(err.Error(), "flush nat masquerade: final-flush-fail") {
		t.Fatalf("expected final flush error, got %v", err)
	}
}

func TestDisableDevMasquerade_FinalFlushError(t *testing.T) {
	cfg := DefaultConfig()
	fc := &fakeConn{}

	// Pre-create BOTH v4 and v6 NAT tables and postrouting chains
	// so helper ensure* functions don't call Flush().
	tbl4 := &nft.Table{Family: nft.TableFamilyIPv4, Name: cfg.TableNat4Name}
	ch4 := &nft.Chain{Table: tbl4, Name: cfg.PostroutingChainName}
	fc.AddTable(tbl4)
	fc.AddChain(ch4)

	tbl6 := &nft.Table{Family: nft.TableFamilyIPv6, Name: cfg.TableNat6Name}
	ch6 := &nft.Chain{Table: tbl6, Name: cfg.PostroutingChainName}
	fc.AddTable(tbl6)
	fc.AddChain(ch6)

	// Now fail the ONLY remaining Flush() (the final one).
	fc.nextFlushErr = errors.New("unmasq-flush-fail")

	b, _ := NewBackendWithConfigAndConn(fc, cfg)
	err := b.DisableDevMasquerade("eth0")
	if err == nil || !strings.Contains(err.Error(), "flush nat unmasq: unmasq-flush-fail") {
		t.Fatalf("expected unmasq flush error, got %v", err)
	}
}

func TestForward_DockerUser_GenericFlushError(t *testing.T) {
	// Docker path present; generic error (not EOPNOTSUPP) should bubble with docker-user prefix.
	cfg := DefaultConfig()
	cfg.PreferDockerUser = true
	fc := &fakeConn{}
	tbl4 := &nft.Table{Family: nft.TableFamilyIPv4, Name: "filter"}
	chUsr4 := &nft.Chain{Table: tbl4, Name: "DOCKER-USER"}
	fc.AddTable(tbl4)
	fc.AddChain(chUsr4)
	fc.nextFlushErr = errors.New("boom")

	b, _ := NewBackendWithConfigAndConn(fc, cfg)
	err := b.EnableForwardingFromTunToDev("tun0", "eth0")
	if err == nil || !strings.Contains(err.Error(), "flush docker-user: boom") {
		t.Fatalf("expected docker-user flush error, got %v", err)
	}
}

func TestForward_InetFallback_ConntrackMissingAnnotated(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PreferDockerUser = false
	fc := &fakeConn{}

	// Pre-create inet table and forward chain so ensureTableFlushed/ensureBaseChainFlushed
	// do NOT call Flush() (they will find existing objects and return).
	tbl := &nft.Table{Family: nft.TableFamilyINet, Name: cfg.TableInetName}
	ch := &nft.Chain{Table: tbl, Name: cfg.ForwardChainName}
	fc.AddTable(tbl)
	fc.AddChain(ch)

	// Make the *final* Flush() fail like missing conntrack.
	fc.nextFlushErr = syscall.EOPNOTSUPP

	b, _ := NewBackendWithConfigAndConn(fc, cfg)
	err := b.EnableForwardingFromTunToDev("tun0", "eth0")
	if err == nil {
		t.Fatalf("expected annotated inet flush error")
	}
	s := strings.ToLower(err.Error())
	if !strings.Contains(s, "flush inet forward") || !strings.Contains(s, "conntrack") {
		t.Fatalf("expected annotated inet flush error, got: %v", err)
	}
}

func TestEnableDisableForwarding_Aliases(t *testing.T) {
	// Cover the Dev<->Tun alias methods explicitly.
	cfg := DefaultConfig()
	cfg.PreferDockerUser = false
	fc := &fakeConn{}
	b, _ := NewBackendWithConfigAndConn(fc, cfg)

	if err := b.EnableForwardingFromDevToTun("tunX", "ethX"); err != nil {
		t.Fatalf("EnableForwardingFromDevToTun: %v", err)
	}
	ch := findChain(t, fc, nft.TableFamilyINet, cfg.TableInetName, cfg.ForwardChainName)
	if !hasRuleWithTag(fc, ch, "tungo:fwd iif=tunX oif=ethX") ||
		!hasRuleWithTag(fc, ch, "tungo:fwdret iif=ethX oif=tunX") {
		t.Fatalf("alias enable did not install rules")
	}
	if err := b.DisableForwardingFromDevToTun("tunX", "ethX"); err != nil {
		t.Fatalf("DisableForwardingFromDevToTun: %v", err)
	}
	if rulesCount(fc, ch) != 0 {
		t.Fatalf("alias disable did not remove rules")
	}
}

func TestZstr_NULTerminated(t *testing.T) {
	b := zstr("abc")
	if len(b) != 4 || b[3] != 0x00 {
		t.Fatalf("zstr must be NUL-terminated, got %v", b)
	}
}

func TestNewBackendAndWithConfig_Smoke(t *testing.T) {
	// Calling those constructors at least executes their lines.
	// They may fail on unusual environments; either result is acceptable.
	if b, err := NewBackendWithConfig(DefaultConfig()); err == nil {
		_ = b.Close()
	}
	if b, err := NewBackend(); err == nil {
		_ = b.Close()
	}
}
