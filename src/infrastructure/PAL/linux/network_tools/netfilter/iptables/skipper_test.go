package iptables

import (
	"errors"
	"reflect"
	"testing"
)

// --- Mocks with required prefix "DefaultSkipper" ---

// SkipperCommanderMock implements PAL.Commander.
type SkipperCommanderMock struct {
	calls []struct {
		method string
		bin    string
		args   []string
	}
	// key => error for CombinedOutput
	resp map[string]error
}

func (m *SkipperCommanderMock) key(method, bin string, args []string) string {
	k := method + "|" + bin + "|"
	for i, a := range args {
		if i > 0 {
			k += " "
		}
		k += a
	}
	return k
}

func (m *SkipperCommanderMock) record(method, bin string, args []string) {
	cp := append([]string{}, args...)
	m.calls = append(m.calls, struct {
		method string
		bin    string
		args   []string
	}{method, bin, cp})
}

func (m *SkipperCommanderMock) setCombined(bin string, args []string, err error) {
	if m.resp == nil {
		m.resp = map[string]error{}
	}
	m.resp[m.key("CombinedOutput", bin, args)] = err
}

func (m *SkipperCommanderMock) CombinedOutput(bin string, args ...string) ([]byte, error) {
	m.record("CombinedOutput", bin, args)
	if err, ok := m.resp[m.key("CombinedOutput", bin, args)]; ok {
		return nil, err
	}
	return nil, nil
}
func (m *SkipperCommanderMock) Output(bin string, args ...string) ([]byte, error) {
	m.record("Output", bin, args)
	return nil, nil
}
func (m *SkipperCommanderMock) Run(bin string, args ...string) error {
	m.record("Run", bin, args)
	return nil
}

// SkipperWaitPolicyMock fakes DefaultWaitPolicy.
type SkipperWaitPolicyMock struct {
	argsByFamily map[string][]string
	calls        []string // families seen
}

func (w *SkipperWaitPolicyMock) Args(family string) []string {
	w.calls = append(w.calls, family)
	if a, ok := w.argsByFamily[family]; ok {
		return append([]string{}, a...)
	}
	return []string{"--wait"}
}

// --- Tests ---

func TestSkipper_NoActionOrChain_NoCallAndFalse(t *testing.T) {
	cmd := &SkipperCommanderMock{}
	wp := &SkipperWaitPolicyMock{argsByFamily: map[string][]string{"IPv4": {"--wait"}}}
	s := NewSkipper("ip6tables", wp, cmd)

	if s.CanSkip("iptables") {
		t.Fatal("empty args must return false")
	}
	if s.CanSkip("iptables", "-t", "filter") {
		t.Fatal("no action/chain must return false")
	}
	if len(cmd.calls) != 0 {
		t.Fatalf("CombinedOutput must not be called; got %d calls", len(cmd.calls))
	}
}

func TestSkipper_Add_SkipWhenExists_And_NotSkipWhenMissing(t *testing.T) {
	cmd := &SkipperCommanderMock{}
	wp := &SkipperWaitPolicyMock{argsByFamily: map[string][]string{
		"IPv4": {"--wait"},
		"IPv6": {"--wait", "--wait-interval", "200"},
	}}
	s := NewSkipper("ip6tables", wp, cmd)

	// exists => skip for -A
	check := []string{"--wait", "-C", "CHAIN", "-p", "tcp", "-j", "ACCEPT"}
	cmd.setCombined("iptables", check, nil)
	if !s.CanSkip("iptables", "-A", "CHAIN", "-p", "tcp", "-j", "ACCEPT") {
		t.Fatal("should skip -A when rule exists")
	}

	// not exists => don't skip for -A
	cmd = &SkipperCommanderMock{}
	s = NewSkipper("ip6tables", wp, cmd)
	// no setCombined → CombinedOutput returns nil by default; make it explicit error to simulate not exists
	cmd.setCombined("iptables", check, errors.New("not found"))
	if s.CanSkip("iptables", "-A", "CHAIN", "-p", "tcp", "-j", "ACCEPT") {
		t.Fatal("should NOT skip -A when rule is absent")
	}
}

func TestSkipper_Delete_SkipWhenAbsent_And_NotSkipWhenExists(t *testing.T) {
	cmd := &SkipperCommanderMock{}
	wp := &SkipperWaitPolicyMock{argsByFamily: map[string][]string{
		"IPv4": {"--wait"},
		"IPv6": {"--wait", "--wait-interval", "200"},
	}}
	s := NewSkipper("ip6tables", wp, cmd)

	check := []string{"--wait", "-C", "CHAIN", "-p", "tcp", "-j", "ACCEPT"}

	// absent → CombinedOutput error → skip -D
	cmd.setCombined("iptables", check, errors.New("nope"))
	if !s.CanSkip("iptables", "-D", "CHAIN", "-p", "tcp", "-j", "ACCEPT") {
		t.Fatal("should skip -D when rule absent")
	}

	// exists → CombinedOutput nil → don't skip -D
	cmd = &SkipperCommanderMock{}
	s = NewSkipper("ip6tables", wp, cmd)
	cmd.setCombined("iptables", check, nil)
	if s.CanSkip("iptables", "-D", "CHAIN", "-p", "tcp", "-j", "ACCEPT") {
		t.Fatal("should NOT skip -D when rule exists")
	}
}

func TestSkipper_Insert_PositionIsConsumed_ExistsControlsSkip(t *testing.T) {
	cmd := &SkipperCommanderMock{}
	wp := &SkipperWaitPolicyMock{argsByFamily: map[string][]string{
		"IPv4": {"--wait"},
		"IPv6": {"--wait", "--wait-interval", "200"},
	}}
	s := NewSkipper("ip6tables", wp, cmd)

	// ensure position "1" is ignored in check building
	check := []string{"--wait", "-C", "CHAIN", "-p", "udp", "--dport", "53", "-j", "ACCEPT"}
	cmd.setCombined("iptables", check, nil) // exists

	if !s.CanSkip("iptables", "-I", "CHAIN", "1", "-p", "udp", "--dport", "53", "-j", "ACCEPT") {
		t.Fatal("should skip -I when equivalent rule exists")
	}

	// now absent => don't skip
	cmd = &SkipperCommanderMock{}
	s = NewSkipper("ip6tables", wp, cmd)
	cmd.setCombined("iptables", check, errors.New("no match"))
	if s.CanSkip("iptables", "-I", "CHAIN", "1", "-p", "udp", "--dport", "53", "-j", "ACCEPT") {
		t.Fatal("should NOT skip -I when rule absent")
	}
}

func TestSkipper_FamilyDetection_IPv6_ByNameAndByExactBin(t *testing.T) {
	// Case 1: by name contains "ip6tables"
	cmd := &SkipperCommanderMock{}
	wp := &SkipperWaitPolicyMock{argsByFamily: map[string][]string{
		"IPv4": {"--wait"},
		"IPv6": {"--wait", "--wait-interval", "200"},
	}}
	s := NewSkipper("/usr/sbin/ip6tables", wp, cmd)

	check6 := []string{"--wait", "--wait-interval", "200", "-C", "C6", "-j", "ACCEPT"}
	cmd.setCombined("ip6tables-nft", check6, nil)
	if !s.CanSkip("ip6tables-nft", "-A", "C6", "-j", "ACCEPT") {
		t.Fatal("IPv6 family by name should use IPv6 wait args and skip when exists")
	}
	if got := wp.calls; len(got) == 0 || got[0] != "IPv6" {
		t.Fatalf("DefaultWaitPolicy.Args should be called with IPv6, got %v", got)
	}

	// Case 2: by exact bin equality with v6bin
	cmd = &SkipperCommanderMock{}
	wp = &SkipperWaitPolicyMock{argsByFamily: map[string][]string{
		"IPv6": {"--wait", "--wait-interval", "300"},
	}}
	s = NewSkipper("/usr/sbin/ip6tables", wp, cmd)
	check6b := []string{"--wait", "--wait-interval", "300", "-C", "C6", "-j", "ACCEPT"}
	cmd.setCombined("/usr/sbin/ip6tables", check6b, nil)
	if !s.CanSkip("/usr/sbin/ip6tables", "-A", "C6", "-j", "ACCEPT") {
		t.Fatal("IPv6 family by exact bin should skip when exists")
	}
}

func TestSkipper_TablePropagation_And_Order(t *testing.T) {
	cmd := &SkipperCommanderMock{}
	wp := &SkipperWaitPolicyMock{argsByFamily: map[string][]string{
		"IPv4": {"--wait", "--wait-interval", "123"},
	}}
	s := NewSkipper("ip6tables", wp, cmd)

	// Build expectation: [wait*] -t nat -C CHAIN spec...
	expect := []string{"--wait", "--wait-interval", "123", "-t", "nat", "-C", "C", "-p", "tcp", "-j", "MASQUERADE"}
	cmd.setCombined("iptables", expect, nil)

	ok := s.CanSkip("iptables", "-t", "nat", "-A", "C", "-p", "tcp", "-j", "MASQUERADE")
	if !ok {
		t.Fatal("should skip -A when exists with table=nat")
	}

	// Verify the actual called args order exactly matches expectation
	if len(cmd.calls) != 1 {
		t.Fatalf("expected 1 CombinedOutput call, got %d", len(cmd.calls))
	}
	call := cmd.calls[0]
	if call.method != "CombinedOutput" || call.bin != "iptables" || !reflect.DeepEqual(call.args, expect) {
		t.Fatalf("unexpected call:\nwant bin=%q args=%v\n got bin=%q args=%v",
			"iptables", expect, call.bin, call.args)
	}
}
