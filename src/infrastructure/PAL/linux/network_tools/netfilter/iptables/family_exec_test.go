package iptables

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// -------------------- Mocks --------------------

// FamilyExecCommanderMock implements PAL.Commander.
type FamilyExecCommanderMock struct {
	calls []struct {
		method string
		bin    string
		args   []string
	}
	// key => (output, err) for CombinedOutput
	resp map[string]struct {
		out []byte
		err error
	}
}

func (m *FamilyExecCommanderMock) key(bin string, args []string) string {
	k := bin + "|"
	for i, a := range args {
		if i > 0 {
			k += " "
		}
		k += a
	}
	return k
}

func (m *FamilyExecCommanderMock) record(method, bin string, args []string) {
	cp := append([]string{}, args...)
	m.calls = append(m.calls, struct {
		method string
		bin    string
		args   []string
	}{method, bin, cp})
}

func (m *FamilyExecCommanderMock) setCombined(bin string, args []string, out []byte, err error) {
	if m.resp == nil {
		m.resp = map[string]struct {
			out []byte
			err error
		}{}
	}
	m.resp[m.key(bin, args)] = struct {
		out []byte
		err error
	}{out: out, err: err}
}

func (m *FamilyExecCommanderMock) CombinedOutput(bin string, args ...string) ([]byte, error) {
	m.record("CombinedOutput", bin, args)
	if r, ok := m.resp[m.key(bin, args)]; ok {
		return r.out, r.err
	}
	return nil, nil
}
func (m *FamilyExecCommanderMock) Output(bin string, args ...string) ([]byte, error) {
	m.record("Output", bin, args)
	return nil, nil
}
func (m *FamilyExecCommanderMock) Run(bin string, args ...string) error {
	m.record("Run", bin, args)
	return nil
}

// FamilyExecWaitPolicyMock fakes WaitPolicy.
type FamilyExecWaitPolicyMock struct {
	argsByFamily map[string][]string
	calls        []string
}

func (w *FamilyExecWaitPolicyMock) Args(family string) []string {
	w.calls = append(w.calls, family)
	if a, ok := w.argsByFamily[family]; ok {
		return append([]string{}, a...)
	}
	return []string{"--wait"}
}

// FamilyExecSkipperMock fakes Skipper.
type FamilyExecSkipperMock struct {
	// key => shouldSkip
	resp  map[string]bool
	calls []struct {
		bin  string
		args []string
	}
}

func (s *FamilyExecSkipperMock) key(bin string, args []string) string {
	k := bin + "|"
	for i, a := range args {
		if i > 0 {
			k += " "
		}
		k += a
	}
	return k
}
func (s *FamilyExecSkipperMock) CanSkip(bin string, args ...string) bool {
	cp := append([]string{}, args...)
	s.calls = append(s.calls, struct {
		bin  string
		args []string
	}{bin: bin, args: cp})
	if s.resp == nil {
		return false
	}
	return s.resp[s.key(bin, args)]
}

// -------------------- Tests --------------------

func TestFamilyExec_SuccessBothFamilies_NoErrors(t *testing.T) {
	cmd := &FamilyExecCommanderMock{}
	wp := &FamilyExecWaitPolicyMock{argsByFamily: map[string][]string{
		"IPv4": {"--wait"},
		"IPv6": {"--wait", "--wait-interval", "200"},
	}}
	sk := &FamilyExecSkipperMock{}

	exec := NewFamilyExec("iptables", "ip6tables", cmd, wp, sk)

	base := []string{"-t", "filter", "-A", "CHAIN", "-j", "ACCEPT"}

	want4 := append([]string{"--wait"}, base...)
	want6 := append([]string{"--wait", "--wait-interval", "200"}, base...)

	if err := exec.ExecBothFamilies(base, "filter", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cmd.calls) != 2 {
		t.Fatalf("expected 2 CombinedOutput calls, got %d", len(cmd.calls))
	}
	if !reflect.DeepEqual(cmd.calls[0].args, want4) || cmd.calls[0].bin != "iptables" {
		t.Fatalf("IPv4 call mismatch: bin=%q args=%v", cmd.calls[0].bin, cmd.calls[0].args)
	}
	if !reflect.DeepEqual(cmd.calls[1].args, want6) || cmd.calls[1].bin != "ip6tables" {
		t.Fatalf("IPv6 call mismatch: bin=%q args=%v", cmd.calls[1].bin, cmd.calls[1].args)
	}

	if got := strings.Join(wp.calls, ","); got != "IPv4,IPv6" {
		t.Fatalf("WaitPolicy.Args calls order mismatch: %s", got)
	}
}

func TestFamilyExec_IPv6Nat_BestEffort_Suppressed(t *testing.T) {
	cmd := &FamilyExecCommanderMock{}
	wp := &FamilyExecWaitPolicyMock{}
	sk := &FamilyExecSkipperMock{}
	exec := NewFamilyExec("iptables", "ip6tables", cmd, wp, sk)

	base := []string{"-t", "nat", "-A", "POSTROUTING", "-j", "MASQUERADE"}

	// IPv4 OK
	cmd.setCombined("iptables", append([]string{"--wait"}, base...), nil, nil)
	errOut := []byte("Can't initialize ip6tables table `nat`: table `nat` not found")
	cmd.setCombined("ip6tables", append([]string{"--wait"}, base...), errOut, errors.New("boom"))

	if err := exec.ExecBothFamilies(base, "nat", true); err != nil {
		t.Fatalf("NAT66 best-effort should suppress the error, got: %v", err)
	}
}

func TestFamilyExec_IPv6Nat_ErrorNotSuppressed_WhenNotNatOrFlagFalse(t *testing.T) {
	cases := []struct {
		table           string
		natV6BestEffort bool
	}{
		{"filter", true},
		{"nat", false},
	}

	for _, tc := range cases {
		cmd := &FamilyExecCommanderMock{}
		wp := &FamilyExecWaitPolicyMock{}
		sk := &FamilyExecSkipperMock{}
		exec := NewFamilyExec("iptables", "ip6tables", cmd, wp, sk)

		base := []string{"-A", "C", "-j", "ACCEPT"}

		// IPv4 ok
		cmd.setCombined("iptables", append([]string{"--wait"}, base...), nil, nil)
		errOut := []byte("can't initialize ip6tables table nat")
		cmd.setCombined("ip6tables", append([]string{"--wait"}, base...), errOut, errors.New("fail"))

		err := exec.ExecBothFamilies(base, tc.table, tc.natV6BestEffort)
		if err == nil {
			t.Fatalf("expected error for table=%s natV6BestEffort=%v", tc.table, tc.natV6BestEffort)
		}
		if !strings.Contains(err.Error(), "[IPv6]") {
			t.Fatalf("expected IPv6 error to propagate, got: %v", err)
		}
	}
}

func TestFamilyExec_run_SkipperSkips_NoCommanderCall(t *testing.T) {
	cmd := &FamilyExecCommanderMock{}
	wp := &FamilyExecWaitPolicyMock{}
	sk := &FamilyExecSkipperMock{resp: map[string]bool{}}

	exec := NewFamilyExec("iptables", "ip6tables", cmd, wp, sk)

	args := []string{"--wait", "-A", "C", "-j", "ACCEPT"}
	sk.resp[sk.key("iptables", args)] = true

	if out, err := exec.run("iptables", args...); err != nil || out != nil {
		t.Fatalf("run should return (nil,nil) when skipped; got (%v,%v)", out, err)
	}
	if len(cmd.calls) != 0 {
		t.Fatalf("CombinedOutput must not be called when skip=true, got %d calls", len(cmd.calls))
	}
}

func TestFamilyExec_run_EmptyBin_NoCall(t *testing.T) {
	cmd := &FamilyExecCommanderMock{}
	wp := &FamilyExecWaitPolicyMock{}
	sk := &FamilyExecSkipperMock{}

	exec := NewFamilyExec("iptables", "ip6tables", cmd, wp, sk)

	if out, err := exec.run("", "--wait", "-A", "C", "-j", "ACCEPT"); err != nil || out != nil {
		t.Fatalf("empty bin should return (nil,nil), got (%v,%v)", out, err)
	}
	if len(cmd.calls) != 0 {
		t.Fatalf("no calls expected, got %d", len(cmd.calls))
	}
}

func TestFamilyExec_ErrorAggregation_BothFamilies(t *testing.T) {
	cmd := &FamilyExecCommanderMock{}
	wp := &FamilyExecWaitPolicyMock{}
	sk := &FamilyExecSkipperMock{}
	exec := NewFamilyExec("iptables", "ip6tables", cmd, wp, sk)

	base := []string{"-A", "C", "-j", "ACCEPT"}

	cmd.setCombined("iptables", append([]string{"--wait"}, base...), []byte("v4 out"), errors.New("v4 fail"))
	cmd.setCombined("ip6tables", append([]string{"--wait"}, base...), []byte("v6 out"), errors.New("v6 fail"))

	err := exec.ExecBothFamilies(base, "filter", false)
	if err == nil {
		t.Fatal("expected aggregated error")
	}
	if !strings.Contains(err.Error(), "[IPv4]") || !strings.Contains(err.Error(), "[IPv6]") {
		t.Fatalf("aggregated error should include both families; got: %v", err)
	}
}

func TestFamilyExec_ArgsOrderAndPropagation(t *testing.T) {
	cmd := &FamilyExecCommanderMock{}
	wp := &FamilyExecWaitPolicyMock{argsByFamily: map[string][]string{
		"IPv4": {"--wait"},
		"IPv6": {"--wait", "--wait-interval", "123"},
	}}
	sk := &FamilyExecSkipperMock{}

	exec := NewFamilyExec("iptables", "ip6tables", cmd, wp, sk)

	base := []string{"-t", "nat", "-A", "POSTROUTING", "-o", "eth0", "-j", "MASQUERADE"}

	want4 := append([]string{"--wait"}, base...)
	want6 := append([]string{"--wait", "--wait-interval", "123"}, base...)

	cmd.setCombined("iptables", want4, nil, nil)
	cmd.setCombined("ip6tables", want6, nil, nil)

	if err := exec.ExecBothFamilies(base, "nat", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cmd.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(cmd.calls))
	}
	if !reflect.DeepEqual(cmd.calls[0].args, want4) || !reflect.DeepEqual(cmd.calls[1].args, want6) {
		t.Fatalf("args order/propagation mismatch:\nwant4=%v\ngot4 =%v\nwant6=%v\ngot6 =%v",
			want4, cmd.calls[0].args, want6, cmd.calls[1].args)
	}
}

func TestFamilyExec_looksLikeNatUnsupported_Variants(t *testing.T) {
	exec := NewFamilyExec("", "", &FamilyExecCommanderMock{}, &FamilyExecWaitPolicyMock{}, &FamilyExecSkipperMock{})

	trueCases := [][]byte{
		[]byte("table `nat` not found"),
		[]byte("can't initialize ip6tables table `nat`"),
		[]byte("can't initialize ip6tables table nat"),
		[]byte("No chain/target/match by that name"),
	}
	for _, tc := range trueCases {
		if !exec.looksLikeNatUnsupported(tc) {
			t.Fatalf("expected true for %q", string(tc))
		}
	}

	falseCases := [][]byte{
		[]byte("permission denied"),
		[]byte("bad rule (does a matching rule exist?)"),
		[]byte("some other error"),
	}
	for _, tc := range falseCases {
		if exec.looksLikeNatUnsupported(tc) {
			t.Fatalf("expected false for %q", string(tc))
		}
	}
}
