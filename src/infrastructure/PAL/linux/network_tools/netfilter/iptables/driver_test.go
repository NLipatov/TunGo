package iptables

import (
	"errors"
	"reflect"
	"testing"
)

/* ---------------- Mocks ---------------- */

type DriverCommanderMock struct {
	calls []struct {
		bin  string
		args []string
	}
	resp map[string][]cmdResp // key -> queue
}

func (m *DriverCommanderMock) key(bin string, args []string) string {
	k := bin + "|"
	for i, a := range args {
		if i > 0 {
			k += " "
		}
		k += a
	}
	return k
}
func (m *DriverCommanderMock) CombinedOutput(bin string, args ...string) ([]byte, error) {
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
func (m *DriverCommanderMock) Output(_ string, _ ...string) ([]byte, error) { return nil, nil }
func (m *DriverCommanderMock) Run(_ string, _ ...string) error              { return nil }

// helper to enqueue response
func (m *DriverCommanderMock) Set(bin string, args []string, out []byte, err error) {
	if m.resp == nil {
		m.resp = map[string][]cmdResp{}
	}
	key := m.key(bin, args)
	m.resp[key] = append(m.resp[key], cmdResp{out: out, err: err})
}

type DriverWaitPolicyMock struct {
	by map[string][]string
}

func (w *DriverWaitPolicyMock) Args(family string) []string {
	if w == nil || w.by == nil {
		return []string{"--wait"}
	}
	if a, ok := w.by[family]; ok {
		return append([]string{}, a...)
	}
	return []string{"--wait"}
}

type DriverSkipperMock struct{}

func (DriverSkipperMock) CanSkip(_ string, _ ...string) bool { return false }

// small helper
func argsWP(w *DriverWaitPolicyMock, fam, table string, tail ...string) []string {
	a := append([]string{}, w.Args(fam)...)
	if table != "" {
		a = append(a, "-t", table)
	}
	return append(a, tail...)
}

/* --------------- Tests --------------- */

func TestDriver_EnsureOnce_ThenReuse(t *testing.T) {
	cmd := &DriverCommanderMock{}
	wp := &DriverWaitPolicyMock{by: map[string][]string{
		"IPv4": {"--wait", "--wait-interval", "111"},
		"IPv6": {"--wait", "--wait-interval", "222"},
	}}
	v4, v6 := "iptables", "ip6tables"

	// Prepare EnsureAll with "already present" minimal path:
	for _, fam := range []string{"IPv4", "IPv6"} {
		bin := v4
		if fam == "IPv6" {
			bin = v6
		}
		// chains exist -> -S ok
		cmd.Set(bin, argsWP(wp, fam, "filter", "-S", fwdChain), nil, nil)
		cmd.Set(bin, argsWP(wp, fam, "mangle", "-S", mangleChain), nil, nil)
		// hooks exist -> -C ok
		cmd.Set(bin, argsWP(wp, fam, "filter", "-C", "FORWARD", "-j", fwdChain), nil, nil)
		cmd.Set(bin, argsWP(wp, fam, "mangle", "-C", "FORWARD", "-j", mangleChain), nil, nil)
	}

	// Build driver and override deps to deterministic ones
	d := New(v4, v6, cmd)
	d.wait = nil // not used after we replace exec/chains
	d.exec = NewFamilyExec(v4, v6, cmd, wp, DriverSkipperMock{})
	d.chains = NewChains(v4, v6, cmd, wp)

	// 1) call any method (nat is fine) – EnsureAll should run
	if err := d.EnableDevMasquerade("eth0"); err != nil {
		t.Fatalf("EnableDevMasquerade: %v", err)
	}

	// snapshot calls so far
	callsAfterFirst := len(cmd.calls)

	// 2) next call — EnsureAll must NOT run again (chainsReady=true)
	if err := d.DisableDevMasquerade("eth0"); err != nil {
		t.Fatalf("DisableDevMasquerade: %v", err)
	}

	if len(cmd.calls) <= callsAfterFirst {
		t.Fatal("expected more CombinedOutput calls after second API call")
	}

	// sanity: FamilyExec should have produced at least one iptables nat POSTROUTING call
	found := false
	for _, c := range cmd.calls {
		if c.bin == v4 {
			if reflect.DeepEqual(c.args[0:4], []string{"--wait", "--wait-interval", "111", "-t"}) && c.args[4] == "nat" {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("expected at least one IPv4 nat invocation with wait args")
	}
}

func TestDriver_EnsureChains_ErrorStopsExecute(t *testing.T) {
	cmd := &DriverCommanderMock{}
	wp := &DriverWaitPolicyMock{by: map[string][]string{"IPv4": {"--wait"}}}
	v4 := "iptables"

	// Make EnsureAll fail at first ensureChain: -S fails, -N fails with "hard" error
	cmd.Set(v4, argsWP(wp, "IPv4", "filter", "-S", fwdChain), nil, errors.New("no chain"))
	cmd.Set(v4, argsWP(wp, "IPv4", "filter", "-N", fwdChain), []byte("boom"), errors.New("fail"))

	d := New(v4, "", cmd)
	d.exec = NewFamilyExec(v4, "", cmd, wp, DriverSkipperMock{})
	d.chains = NewChains(v4, "", cmd, wp)

	if err := d.EnableDevMasquerade("eth0"); err == nil {
		t.Fatal("expected error from EnsureAll, got nil")
	}

	// Ensure ExecBothFamilies did not run (-A POSTROUTING would be present)
	for _, c := range cmd.calls {
		if c.bin == v4 {
			for i := 0; i < len(c.args)-1; i++ {
				if c.args[i] == "POSTROUTING" && c.args[i-1] == "-A" {
					t.Fatal("ExecBothFamilies should not be called when EnsureAll fails")
				}
			}
		}
	}
}

func TestDriver_NAT_Ipv6ErrorIsBestEffort(t *testing.T) {
	cmd := &DriverCommanderMock{}
	wp := &DriverWaitPolicyMock{by: map[string][]string{
		"IPv4": {"--wait"},
		"IPv6": {"--wait"},
	}}
	v4, v6 := "iptables", "ip6tables"

	// EnsureAll minimal "present"
	for _, fam := range []string{"IPv4", "IPv6"} {
		bin := v4
		if fam == "IPv6" {
			bin = v6
		}
		cmd.Set(bin, argsWP(wp, fam, "filter", "-S", fwdChain), nil, nil)
		cmd.Set(bin, argsWP(wp, fam, "mangle", "-S", mangleChain), nil, nil)
		cmd.Set(bin, argsWP(wp, fam, "filter", "-C", "FORWARD", "-j", fwdChain), nil, nil)
		cmd.Set(bin, argsWP(wp, fam, "mangle", "-C", "FORWARD", "-j", mangleChain), nil, nil)
	}

	// ExecBothFamilies:
	//  - IPv4 nat POSTROUTING ok
	//  - IPv6 nat POSTROUTING returns "can't initialize ip6tables table nat" -> should be ignored
	cmd.Set(v4, argsWP(wp, "IPv4", "nat",
		"-A", "POSTROUTING", "-o", "eth0", "-m", "comment", "--comment", "tungo", "-j", "MASQUERADE"), nil, nil)
	cmd.Set(v6, argsWP(wp, "IPv6", "nat",
		"-A", "POSTROUTING", "-o", "eth0", "-m", "comment", "--comment", "tungo", "-j", "MASQUERADE"),
		[]byte("can't initialize ip6tables table nat"), errors.New("bad"))

	d := New(v4, v6, cmd)
	d.exec = NewFamilyExec(v4, v6, cmd, wp, DriverSkipperMock{})
	d.chains = NewChains(v4, v6, cmd, wp)

	if err := d.EnableDevMasquerade("eth0"); err != nil {
		t.Fatalf("expected nil (best-effort NAT66), got %v", err)
	}
}

func TestDriver_Filter_Ipv6ErrorIsFatal(t *testing.T) {
	cmd := &DriverCommanderMock{}
	wp := &DriverWaitPolicyMock{by: map[string][]string{
		"IPv4": {"--wait"},
		"IPv6": {"--wait"},
	}}
	v4, v6 := "iptables", "ip6tables"

	// EnsureAll minimal "present"
	for _, fam := range []string{"IPv4", "IPv6"} {
		bin := v4
		if fam == "IPv6" {
			bin = v6
		}
		cmd.Set(bin, argsWP(wp, fam, "filter", "-S", fwdChain), nil, nil)
		cmd.Set(bin, argsWP(wp, fam, "mangle", "-S", mangleChain), nil, nil)
		cmd.Set(bin, argsWP(wp, fam, "filter", "-C", "FORWARD", "-j", fwdChain), nil, nil)
		cmd.Set(bin, argsWP(wp, fam, "mangle", "-C", "FORWARD", "-j", mangleChain), nil, nil)
	}

	// call EnableForwardingFromTunToDev (table filter)
	//  - IPv4 ok
	//  - IPv6 error => must bubble up (not nat)
	cmd.Set(v4, argsWP(wp, "IPv4", "filter",
		"-A", fwdChain, "-i", "tun0", "-o", "eth0", "-m", "comment", "--comment", "tungo", "-j", "ACCEPT"), nil, nil)
	cmd.Set(v6, argsWP(wp, "IPv6", "filter",
		"-A", fwdChain, "-i", "tun0", "-o", "eth0", "-m", "comment", "--comment", "tungo", "-j", "ACCEPT"), []byte("x"), errors.New("fail"))

	d := New(v4, v6, cmd)
	d.exec = NewFamilyExec(v4, v6, cmd, wp, DriverSkipperMock{})
	d.chains = NewChains(v4, v6, cmd, wp)

	err := d.EnableForwardingFromTunToDev("tun0", "eth0")
	if err == nil {
		t.Fatal("expected fatal error on IPv6 in filter table")
	}
}

func TestDriver_ConfigureMssClamping_SuccessAndShortCircuitOnError(t *testing.T) {
	// Success path -> two execute calls (out+in), each does v4+v6
	{
		cmd := &DriverCommanderMock{}
		wp := &DriverWaitPolicyMock{by: map[string][]string{"IPv4": {"--wait"}, "IPv6": {"--wait"}}}
		v4, v6 := "iptables", "ip6tables"

		// pre-mark chainsReady to skip EnsureAll (we still need d.chains for Teardown tests below)
		d := New(v4, v6, cmd)
		d.exec = NewFamilyExec(v4, v6, cmd, wp, DriverSkipperMock{})
		d.chains = NewChains(v4, v6, cmd, wp)
		d.chainsReady.Store(true)

		// out rule
		out := []string{"-A", mangleChain, "-o", "eth0", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN",
			"-m", "comment", "--comment", "tungo", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}
		cmd.Set(v4, argsWP(wp, "IPv4", "mangle", out...), nil, nil)
		cmd.Set(v6, argsWP(wp, "IPv6", "mangle", out...), nil, nil)
		// in rule
		in := []string{"-A", mangleChain, "-i", "eth0", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN",
			"-m", "comment", "--comment", "tungo", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}
		cmd.Set(v4, argsWP(wp, "IPv4", "mangle", in...), nil, nil)
		cmd.Set(v6, argsWP(wp, "IPv6", "mangle", in...), nil, nil)

		if err := d.ConfigureMssClamping("eth0"); err != nil {
			t.Fatalf("ConfigureMssClamping success: %v", err)
		}

		if len(cmd.calls) != 4 {
			t.Fatalf("expected exactly 4 CombinedOutput calls (v4 then v6), got %d", len(cmd.calls))
		}
		if cmd.calls[0].bin != v4 || cmd.calls[1].bin != v6 {
			t.Fatalf("expected order v4 -> v6, got %s -> %s", cmd.calls[0].bin, cmd.calls[1].bin)
		}
	}

	// Error on first execute -> second execute must NOT run
	{
		cmd := &DriverCommanderMock{}
		wp := &DriverWaitPolicyMock{by: map[string][]string{"IPv4": {"--wait"}, "IPv6": {"--wait"}}}
		v4, v6 := "iptables", "ip6tables"

		d := New(v4, v6, cmd)
		d.exec = NewFamilyExec(v4, v6, cmd, wp, DriverSkipperMock{})
		d.chains = NewChains(v4, v6, cmd, wp)
		d.chainsReady.Store(true)

		out := []string{"-A", mangleChain, "-o", "eth0", "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN",
			"-m", "comment", "--comment", "tungo", "-j", "TCPMSS", "--clamp-mss-to-pmtu"}
		// make IPv4 fail -> ExecBothFamilies returns error
		cmd.Set(v4, argsWP(wp, "IPv4", "mangle", out...), []byte("oops"), errors.New("fail"))
		// (we don't even need to set IPv6, it won't be reached if v4 already fails inside ExecBothFamilies)

		if err := d.ConfigureMssClamping("eth0"); err == nil {
			t.Fatal("expected error from first execute, got nil")
		}

		if len(cmd.calls) != 2 {
			t.Fatalf("expected exactly 2 CombinedOutput calls (v4 then v6), got %d", len(cmd.calls))
		}
		if cmd.calls[0].bin != v4 || cmd.calls[1].bin != v6 {
			t.Fatalf("expected order v4 -> v6, got %s -> %s", cmd.calls[0].bin, cmd.calls[1].bin)
		}
	}
}

func TestDriver_GetTableFromArgs(t *testing.T) {
	d := &Driver{}
	if got := d.getTableFromArgs([]string{"-A", "X"}); got != "filter" {
		t.Fatalf("default table want filter, got %s", got)
	}
	if got := d.getTableFromArgs([]string{"-t", "mangle", "-A", "X"}); got != "mangle" {
		t.Fatalf("table want mangle, got %s", got)
	}
	if got := d.getTableFromArgs([]string{"-t", "nat", "-A", "X", "-t", "mangle"}); got != "nat" {
		t.Fatalf("getTableFromArgs must take first -t occurrence, got %s", got)
	}
}

func TestDriver_TeardownChains_Delegates(t *testing.T) {
	cmd := &DriverCommanderMock{}
	wp := &DriverWaitPolicyMock{by: map[string][]string{"IPv4": {"--wait"}, "IPv6": {"--wait"}}}
	v4, v6 := "iptables", "ip6tables"

	for _, fam := range []string{"IPv4", "IPv6"} {
		bin := v4
		if fam == "IPv6" {
			bin = v6
		}
		for _, tb := range []string{"filter", "mangle"} {
			// unhook fwdChain
			d1 := argsWP(wp, fam, tb, "-D", "FORWARD", "-j", fwdChain)
			cmd.Set(bin, d1, nil, nil)
			cmd.Set(bin, d1, []byte("no chain/target/match by that name"), errors.New("stop"))
			// unhook mangleChain
			d2 := argsWP(wp, fam, tb, "-D", "FORWARD", "-j", mangleChain)
			cmd.Set(bin, d2, nil, nil)
			cmd.Set(bin, d2, []byte("no chain/target/match by that name"), errors.New("stop"))

			ch := fwdChain
			if tb == "mangle" {
				ch = mangleChain
			}
			flush := argsWP(wp, fam, tb, "-F", ch)
			del := argsWP(wp, fam, tb, "-X", ch)
			cmd.Set(bin, flush, nil, errors.New("ignored"))
			cmd.Set(bin, del, nil, nil)
		}
	}

	d := New(v4, v6, cmd)
	d.exec = NewFamilyExec(v4, v6, cmd, wp, DriverSkipperMock{})
	d.chains = NewChains(v4, v6, cmd, wp)

	if err := d.TeardownChains(); err != nil {
		t.Fatalf("TeardownChains: %v", err)
	}
}
