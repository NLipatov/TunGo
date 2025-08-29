package iptables

import (
	"errors"
	"reflect"
	"sync"
	"testing"
)

// --- mock PAL with required prefix "DefaultWaitPolicy" ---

type WaitPolicyCommanderMock struct {
	mu    sync.Mutex
	calls []struct {
		method string // "CombinedOutput" | "Output" | "Run"
		bin    string
		args   []string
	}
	// key = method + "|" + bin + "|" + strings.Join(args, " ")
	coResp map[string]error
	outBuf map[string][]byte // optional payload
}

func (m *WaitPolicyCommanderMock) record(method, bin string, args []string) {
	m.mu.Lock()
	m.calls = append(m.calls, struct {
		method string
		bin    string
		args   []string
	}{method, bin, append([]string{}, args...)})
	m.mu.Unlock()
}

func (m *WaitPolicyCommanderMock) key(method, bin string, args []string) string {
	s := method + "|" + bin + "|"
	for i, a := range args {
		if i > 0 {
			s += " "
		}
		s += a
	}
	return s
}

func (m *WaitPolicyCommanderMock) setCombined(bin string, args []string, out []byte, err error) {
	if m.coResp == nil {
		m.coResp = map[string]error{}
	}
	if m.outBuf == nil {
		m.outBuf = map[string][]byte{}
	}
	k := m.key("CombinedOutput", bin, args)
	m.coResp[k] = err
	m.outBuf[k] = out
}

// ---- Commander interface ----

func (m *WaitPolicyCommanderMock) CombinedOutput(bin string, args ...string) ([]byte, error) {
	m.record("CombinedOutput", bin, args)
	k := m.key("CombinedOutput", bin, args)
	if err, ok := m.coResp[k]; ok {
		return m.outBuf[k], err
	}
	return nil, nil
}

func (m *WaitPolicyCommanderMock) Output(bin string, args ...string) ([]byte, error) {
	m.record("Output", bin, args)
	// Not used by DefaultWaitPolicy; return zero-values.
	return nil, nil
}

func (m *WaitPolicyCommanderMock) Run(bin string, args ...string) error {
	m.record("Run", bin, args)
	// Not used by DefaultWaitPolicy; return nil.
	return nil
}

func TestWaitPolicy_Constructors(t *testing.T) {
	m := &WaitPolicyCommanderMock{}
	wp := NewWaitPolicy("iptables", "ip6tables", m)
	if wp.waitMs != 200 {
		t.Fatalf("default waitMs want 200, got %d", wp.waitMs)
	}
	wp2 := NewWaitPolicyWithWaitMS("iptables", "ip6tables", m, 350)
	if wp2.waitMs != 350 {
		t.Fatalf("custom waitMs want 350, got %d", wp2.waitMs)
	}
}

func TestWaitPolicy_SetInterval(t *testing.T) {
	m := &WaitPolicyCommanderMock{}
	wp := NewWaitPolicy("iptables", "ip6tables", m)
	wp.SetInterval(500)
	if wp.waitMs != 500 {
		t.Fatalf("want waitMs=500, got %d", wp.waitMs)
	}
	wp.SetInterval(0) // should be ignored
	if wp.waitMs != 500 {
		t.Fatalf("ms<=0 must not change, got %d", wp.waitMs)
	}
}

func TestWaitPolicy_Detect_AllCombosAndOnce(t *testing.T) {
	m := &WaitPolicyCommanderMock{}
	// First run: v4 success, v6 error
	m.setCombined("iptables", []string{"--wait", "--wait-interval", "1", "-S"}, nil, nil)
	m.setCombined("ip6tables", []string{"--wait", "--wait-interval", "1", "-S"}, nil, errors.New("nope"))

	wp := NewWaitPolicy("iptables", "ip6tables", m)
	wp.Detect()
	if !wp.v4ok || wp.v6ok {
		t.Fatalf("after first detect: v4ok=true, v6ok=false; got v4ok=%v v6ok=%v", wp.v4ok, wp.v6ok)
	}

	// Change responses to success for both, then call Detect again — sync.Once prevents re-detect
	m.setCombined("iptables", []string{"--wait", "--wait-interval", "1", "-S"}, nil, nil)
	m.setCombined("ip6tables", []string{"--wait", "--wait-interval", "1", "-S"}, nil, nil)
	wp.Detect()

	// State must remain unchanged
	if !wp.v4ok || wp.v6ok {
		t.Fatalf("sync.Once should keep previous state, got v4ok=%v v6ok=%v", wp.v4ok, wp.v6ok)
	}

	// Ensure only CombinedOutput was called exactly twice
	co := 0
	for _, c := range m.calls {
		if c.method != "CombinedOutput" {
			t.Fatalf("Detect should not call %s", c.method)
		}
		co++
	}
	if co != 2 {
		t.Fatalf("Detect should call CombinedOutput twice; got %d", co)
	}
}

func TestWaitPolicy_Detect_EmptyBins(t *testing.T) {
	m := &WaitPolicyCommanderMock{}
	wp := NewWaitPolicy("", "", m)
	wp.Detect()
	if wp.v4ok || wp.v6ok {
		t.Fatalf("empty bins must yield false flags, got v4ok=%v v6ok=%v", wp.v4ok, wp.v6ok)
	}
	if len(m.calls) != 0 {
		t.Fatalf("empty bins should not call PAL, calls=%d", len(m.calls))
	}
}

func TestWaitPolicy_Args_Variants(t *testing.T) {
	m := &WaitPolicyCommanderMock{}
	// v4 ok, v6 not ok
	m.setCombined("iptables", []string{"--wait", "--wait-interval", "1", "-S"}, nil, nil)
	m.setCombined("ip6tables", []string{"--wait", "--wait-interval", "1", "-S"}, nil, errors.New("nope"))

	wp := NewWaitPolicy("iptables", "ip6tables", m)
	wp.SetInterval(321)
	got4 := wp.Args("IPv4")
	want4 := []string{"--wait", "--wait-interval", "321"}
	if !reflect.DeepEqual(got4, want4) {
		t.Fatalf("IPv4 args mismatch: want %v, got %v", want4, got4)
	}
	got6 := wp.Args("IPv6")
	want6 := []string{"--wait"}
	if !reflect.DeepEqual(got6, want6) {
		t.Fatalf("IPv6 args mismatch: want %v, got %v", want6, got6)
	}

	// Args should not trigger more PAL calls after first Detect
	calls := len(m.calls)
	_ = wp.Args("IPv4")
	_ = wp.Args("IPv6")
	if len(m.calls) != calls {
		t.Fatalf("Args should not re-detect; extra calls: %d->%d", calls, len(m.calls))
	}
}
