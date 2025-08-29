package iptables

import (
	"errors"
	"reflect"
	"testing"
)

// ---------- Mocks ----------

type cmdResp struct {
	out []byte
	err error
}

type ChainsCommanderMock struct {
	calls []struct {
		bin  string
		args []string
	}
	// key -> queue of responses (pop FIFO)
	resp map[string][]cmdResp
}

func (m *ChainsCommanderMock) key(bin string, args []string) string {
	k := bin + "|"
	for i, a := range args {
		if i > 0 {
			k += " "
		}
		k += a
	}
	return k
}

func (m *ChainsCommanderMock) CombinedOutput(bin string, args ...string) ([]byte, error) {
	cp := append([]string{}, args...)
	m.calls = append(m.calls, struct {
		bin  string
		args []string
	}{bin, cp})

	if m.resp == nil {
		return nil, nil
	}
	key := m.key(bin, args)
	q := m.resp[key]
	if len(q) == 0 {
		return nil, nil
	}
	r := q[0]
	m.resp[key] = q[1:]
	return r.out, r.err
}

func (m *ChainsCommanderMock) Output(_ string, _ ...string) ([]byte, error) { return nil, nil }
func (m *ChainsCommanderMock) Run(_ string, _ ...string) error              { return nil }

func (m *ChainsCommanderMock) Set(bin string, args []string, out []byte, err error) {
	if m.resp == nil {
		m.resp = map[string][]cmdResp{}
	}
	key := m.key(bin, args)
	m.resp[key] = append(m.resp[key], cmdResp{out: out, err: err})
}

type WaitPolicyMock struct {
	by map[string][]string // family -> args
}

func (w *WaitPolicyMock) Args(family string) []string {
	if w == nil || w.by == nil {
		return []string{"--wait"}
	}
	if a, ok := w.by[family]; ok {
		return append([]string{}, a...)
	}
	return []string{"--wait"}
}

// ---------- Helpers ----------

func args(w WaitPolicy, family, table string, tail ...string) []string {
	base := append([]string{}, w.Args(family)...)
	if table != "" {
		base = append(base, "-t", table)
	}
	return append(base, tail...)
}

// ---------- Tests ----------

func TestChains_EnsureAll_CreateAndHook_WhenMissing(t *testing.T) {
	cmd := &ChainsCommanderMock{}
	wp := &WaitPolicyMock{by: map[string][]string{
		"IPv4": {"--wait", "--wait-interval", "111"},
		"IPv6": {"--wait", "--wait-interval", "222"},
	}}
	c := NewChains("iptables", "ip6tables", cmd, wp)

	// ensureChain (4 chains): first -S -> error, then -N -> ok
	for _, fam := range []string{"IPv4", "IPv6"} {
		bin := "iptables"
		if fam == "IPv6" {
			bin = "ip6tables"
		}
		// filter/fwd
		cmd.Set(bin, args(wp, fam, "filter", "-S", fwdChain), nil, errors.New("no chain"))
		cmd.Set(bin, args(wp, fam, "filter", "-N", fwdChain), nil, nil)
		// mangle/mangleChain
		cmd.Set(bin, args(wp, fam, "mangle", "-S", mangleChain), nil, errors.New("no chain"))
		cmd.Set(bin, args(wp, fam, "mangle", "-N", mangleChain), nil, nil)
	}

	// ensureHookAppend (4 hooks): -C -> error, then -A -> ok
	for _, fam := range []string{"IPv4", "IPv6"} {
		bin := "iptables"
		if fam == "IPv6" {
			bin = "ip6tables"
		}
		// filter/FORWARD -> fwdChain
		cmd.Set(bin, args(wp, fam, "filter", "-C", "FORWARD", "-j", fwdChain), nil, errors.New("no hook"))
		cmd.Set(bin, args(wp, fam, "filter", "-A", "FORWARD", "-j", fwdChain), nil, nil)

		// mangle/FORWARD -> mangleChain
		cmd.Set(bin, args(wp, fam, "mangle", "-C", "FORWARD", "-j", mangleChain), nil, errors.New("no hook"))
		cmd.Set(bin, args(wp, fam, "mangle", "-A", "FORWARD", "-j", mangleChain), nil, nil)
	}

	if err := c.EnsureAll(fwdChain, mangleChain); err != nil {
		t.Fatalf("EnsureAll unexpected error: %v", err)
	}

	// 4 chains * (S + N) * 2 families = 16 calls? (actually 4 chains total across both families)
	// Correct count: 4 chains * 2 calls = 8, plus 4 hooks * 2 calls = 8 => 16.
	if got := len(cmd.calls); got != 16 {
		t.Fatalf("expected 16 CombinedOutput calls, got %d", got)
	}
}

func TestChains_EnsureAll_Existing_NoOps(t *testing.T) {
	cmd := &ChainsCommanderMock{}
	wp := &WaitPolicyMock{by: map[string][]string{
		"IPv4": {"--wait", "--wait-interval", "111"},
		"IPv6": {"--wait", "--wait-interval", "222"},
	}}
	c := NewChains("iptables", "ip6tables", cmd, wp)

	// Chains already exist: only -S returns nil (no -N)
	for _, fam := range []string{"IPv4", "IPv6"} {
		bin := "iptables"
		if fam == "IPv6" {
			bin = "ip6tables"
		}
		cmd.Set(bin, args(wp, fam, "filter", "-S", fwdChain), nil, nil)
		cmd.Set(bin, args(wp, fam, "mangle", "-S", mangleChain), nil, nil)
	}

	// Hooks already present: only -C returns nil (no -A)
	for _, fam := range []string{"IPv4", "IPv6"} {
		bin := "iptables"
		if fam == "IPv6" {
			bin = "ip6tables"
		}
		cmd.Set(bin, args(wp, fam, "filter", "-C", "FORWARD", "-j", fwdChain), nil, nil)
		cmd.Set(bin, args(wp, fam, "mangle", "-C", "FORWARD", "-j", mangleChain), nil, nil)
	}

	if err := c.EnsureAll(fwdChain, mangleChain); err != nil {
		t.Fatalf("EnsureAll unexpected error: %v", err)
	}

	// Only 8 checks: 4 x -S + 4 x -C
	if got := len(cmd.calls); got != 8 {
		t.Fatalf("expected 8 CombinedOutput calls, got %d", got)
	}
}

func TestChains_ensureChain_CreateErrButAlreadyExists_IsOk(t *testing.T) {
	cmd := &ChainsCommanderMock{}
	wp := &WaitPolicyMock{by: map[string][]string{"IPv4": {"--wait"}}}
	c := NewChains("iptables", "", cmd, wp)

	// -S fails (not exists), -N fails but says "chain already exists" in output
	cmd.Set("iptables", args(wp, "IPv4", "filter", "-S", "MY"), nil, errors.New("no"))
	cmd.Set("iptables", args(wp, "IPv4", "filter", "-N", "MY"), []byte("Chain AlReAdY ExIsTs"), errors.New("oops"))

	if err := c.ensureChain("IPv4", "iptables", "filter", "MY"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestChains_ensureChain_CreateHardError_Propagates(t *testing.T) {
	cmd := &ChainsCommanderMock{}
	wp := &WaitPolicyMock{by: map[string][]string{"IPv4": {"--wait"}}}
	c := NewChains("iptables", "", cmd, wp)

	cmd.Set("iptables", args(wp, "IPv4", "filter", "-S", "MY"), nil, errors.New("no"))
	cmd.Set("iptables", args(wp, "IPv4", "filter", "-N", "MY"), []byte("boom"), errors.New("fail"))

	if err := c.ensureChain("IPv4", "iptables", "filter", "MY"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestChains_ensureHookAppend_Failure_Propagates(t *testing.T) {
	cmd := &ChainsCommanderMock{}
	wp := &WaitPolicyMock{by: map[string][]string{"IPv6": {"--wait", "--wait-interval", "2"}}}
	c := NewChains("", "ip6tables", cmd, wp)

	cmd.Set("ip6tables", args(wp, "IPv6", "filter", "-C", "FORWARD", "-j", "C"), nil, errors.New("missing"))
	cmd.Set("ip6tables", args(wp, "IPv6", "filter", "-A", "FORWARD", "-j", "C"), []byte("denied"), errors.New("boom"))

	if err := c.ensureHookAppend("IPv6", "ip6tables", "filter", "FORWARD", "C"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestChains_unhookAll_DuplicatesThenNoSuch(t *testing.T) {
	cmd := &ChainsCommanderMock{}
	wp := &WaitPolicyMock{by: map[string][]string{"IPv4": {"--wait"}}}
	c := NewChains("iptables", "", cmd, wp)

	del := args(wp, "IPv4", "filter", "-D", "FORWARD", "-j", "C")
	// two successful deletions, then "no such" to stop
	cmd.Set("iptables", del, nil, nil)
	cmd.Set("iptables", del, nil, nil)
	cmd.Set("iptables", del, []byte("no chain/target/match by that name"), errors.New("stop"))

	if err := c.unhookAll("IPv4", "iptables", "filter", "FORWARD", "C"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// exactly three -D invocations
	count := 0
	for _, call := range cmd.calls {
		if call.bin == "iptables" && reflect.DeepEqual(call.args, del) {
			count++
		}
	}
	if count != 3 {
		t.Fatalf("expected 3 delete tries, got %d", count)
	}
}

func TestChains_unhookAll_ErrorOnDeleteIsReturned(t *testing.T) {
	cmd := &ChainsCommanderMock{}
	wp := &WaitPolicyMock{by: map[string][]string{"IPv4": {"--wait"}}}
	c := NewChains("iptables", "", cmd, wp)

	del := args(wp, "IPv4", "filter", "-D", "FORWARD", "-j", "C")
	cmd.Set("iptables", del, []byte("other error"), errors.New("fail"))

	if err := c.unhookAll("IPv4", "iptables", "filter", "FORWARD", "C"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestChains_dropChain_Various(t *testing.T) {
	t.Run("success_with_flush_ignored", func(t *testing.T) {
		cmd := &ChainsCommanderMock{}
		wp := &WaitPolicyMock{by: map[string][]string{"IPv4": {"--wait"}}}
		c := NewChains("iptables", "", cmd, wp)

		flush := args(wp, "IPv4", "filter", "-F", "C")
		del := args(wp, "IPv4", "filter", "-X", "C")
		cmd.Set("iptables", flush, nil, errors.New("ignored"))
		cmd.Set("iptables", del, nil, nil)

		if err := c.dropChain("IPv4", "iptables", "filter", "C"); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	})

	t.Run("delete_noSuch_is_ignored", func(t *testing.T) {
		cmd := &ChainsCommanderMock{}
		wp := &WaitPolicyMock{by: map[string][]string{"IPv4": {"--wait"}}}
		c := NewChains("iptables", "", cmd, wp)

		del := args(wp, "IPv4", "mangle", "-X", "C")
		cmd.Set("iptables", del, []byte("does a matching rule exist"), errors.New("nope"))

		if err := c.dropChain("IPv4", "iptables", "mangle", "C"); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	})

	t.Run("delete_real_error_is_returned", func(t *testing.T) {
		cmd := &ChainsCommanderMock{}
		wp := &WaitPolicyMock{by: map[string][]string{"IPv4": {"--wait"}}}
		c := NewChains("iptables", "", cmd, wp)

		del := args(wp, "IPv4", "mangle", "-X", "C")
		cmd.Set("iptables", del, []byte("permission denied"), errors.New("fail"))

		if err := c.dropChain("IPv4", "iptables", "mangle", "C"); err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestChains_Teardown_EndToEnd(t *testing.T) {
	cmd := &ChainsCommanderMock{}
	wp := &WaitPolicyMock{by: map[string][]string{
		"IPv4": {"--wait"},
		"IPv6": {"--wait", "--wait-interval", "2"},
	}}
	c := NewChains("iptables", "ip6tables", cmd, wp)

	// Unhook all four edges: each has one deletion then "no such"
	for _, fam := range []string{"IPv4", "IPv6"} {
		bin := "iptables"
		if fam == "IPv6" {
			bin = "ip6tables"
		}
		for _, tb := range []string{"filter", "mangle"} {
			delFwd := args(wp, fam, tb, "-D", "FORWARD", "-j", fwdChain)
			delMng := args(wp, fam, tb, "-D", "FORWARD", "-j", mangleChain)

			// For filter/fwdChain
			cmd.Set(bin, delFwd, nil, nil)
			cmd.Set(bin, delFwd, []byte("no chain/target/match by that name"), errors.New("stop"))
			// For mangle/mangleChain
			cmd.Set(bin, delMng, nil, nil)
			cmd.Set(bin, delMng, []byte("no chain/target/match by that name"), errors.New("stop"))
		}
	}

	// Drops: flush ignored; delete ok
	for _, fam := range []string{"IPv4", "IPv6"} {
		bin := "iptables"
		if fam == "IPv6" {
			bin = "ip6tables"
		}
		for _, tb := range []string{"filter", "mangle"} {
			chain := fwdChain
			if tb == "mangle" {
				chain = mangleChain
			}
			flush := args(wp, fam, tb, "-F", chain)
			del := args(wp, fam, tb, "-X", chain)
			cmd.Set(bin, flush, nil, errors.New("ignored"))
			cmd.Set(bin, del, nil, nil)
		}
	}

	if err := c.Teardown(); err != nil {
		t.Fatalf("unexpected Teardown error: %v", err)
	}
}

func TestChains_BinEmpty_EarlyReturn(t *testing.T) {
	cmd := &ChainsCommanderMock{}
	wp := &WaitPolicyMock{}
	c := NewChains("", "", cmd, wp)

	// All should early-return nil and produce no calls
	if err := c.ensureChain("IPv4", "", "filter", "X"); err != nil {
		t.Fatalf("ensureChain: %v", err)
	}
	if err := c.ensureHookAppend("IPv6", "", "mangle", "P", "C"); err != nil {
		t.Fatalf("ensureHookAppend: %v", err)
	}
	if err := c.unhookAll("IPv4", "", "filter", "P", "C"); err != nil {
		t.Fatalf("unhookAll: %v", err)
	}
	if err := c.dropChain("IPv6", "", "mangle", "X"); err != nil {
		t.Fatalf("dropChain: %v", err)
	}
	if len(cmd.calls) != 0 {
		t.Fatalf("expected 0 calls, got %d", len(cmd.calls))
	}
}

func TestChains_noSuchErr_Variants(t *testing.T) {
	c := &Chains{}
	cases := []string{
		"no chain/target/match by that name",
		"bad rule (does a matching rule exist",
		"does a matching rule exist",
		"NO SUCH FILE OR DIRECTORY",
	}
	for _, s := range cases {
		if !c.noSuchErr(s) {
			t.Fatalf("expected true for %q", s)
		}
	}
}
