package ifconfig

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
)

// fakeCommander lets us control CombinedOutput behavior.
type fakeCommander struct {
	wantName string
	wantArgs []string
	out      []byte
	err      error
	called   bool
}

func (f *fakeCommander) CombinedOutput(name string, args ...string) ([]byte, error) {
	f.called = true
	if name != f.wantName {
		return nil, fmt.Errorf("unexpected command name: got %q, want %q", name, f.wantName)
	}
	if len(args) != len(f.wantArgs) {
		return nil, fmt.Errorf("unexpected args length: got %v, want %v", args, f.wantArgs)
	}
	for i := range args {
		if args[i] != f.wantArgs[i] {
			return nil, fmt.Errorf("unexpected arg[%d]: got %q, want %q", i, args[i], f.wantArgs[i])
		}
	}
	return f.out, f.err
}

// We only need CombinedOutput on this wrapper.
func (f *fakeCommander) Output(_ string, _ ...string) ([]byte, error) { return nil, nil }
func (f *fakeCommander) Run(_ string, _ ...string) error              { return nil }

func TestLinkAddrAdd_InvalidCIDR(t *testing.T) {
	w := NewWrapper(&fakeCommander{})
	err := w.LinkAddrAdd("eth0", "not-a-cidr")
	if err == nil || !strings.Contains(err.Error(), "invalid CIDR") {
		t.Fatalf("expected invalid CIDR error, got %v", err)
	}
}

func TestLinkAddrAdd_CommandFails(t *testing.T) {
	cidr := "10.0.1.20/24"
	mask := "255.255.255.0"
	fc := &fakeCommander{
		wantName: "ifconfig",
		wantArgs: []string{"eth0", "inet", "10.0.1.20", "10.0.1.20", "netmask", mask},
		out:      []byte("bad output"),
		err:      errors.New("boom"),
	}
	w := NewWrapper(fc)
	err := w.LinkAddrAdd("eth0", cidr)
	if err == nil {
		t.Fatal("expected error from CombinedOutput, got nil")
	}
	if !strings.Contains(err.Error(), "failed to assign IP to eth0: boom (bad output)") {
		t.Errorf("unexpected error message: %v", err)
	}
	if !fc.called {
		t.Error("expected CombinedOutput to be called")
	}
}

func TestLinkAddrAdd_Success(t *testing.T) {
	cidr := "192.168.0.5/16"
	// prefixToNetmask(16) == 255.255.0.0
	fc := &fakeCommander{
		wantName: "ifconfig",
		wantArgs: []string{"tun10", "inet", "192.168.0.5", "192.168.0.5", "netmask", "255.255.0.0"},
		out:      []byte(""),
		err:      nil,
	}
	w := NewWrapper(fc)
	if err := w.LinkAddrAdd("tun10", cidr); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !fc.called {
		t.Error("expected CombinedOutput to be called")
	}
}

func TestPrefixToNetmask_Valid(t *testing.T) {
	w := NewWrapper(nil)
	cases := []struct {
		pref, want string
	}{
		{"0", "0.0.0.0"},
		{"1", "128.0.0.0"},
		{"8", "255.0.0.0"},
		{"16", "255.255.0.0"},
		{"24", "255.255.255.0"},
		{"32", "255.255.255.255"},
	}
	for _, c := range cases {
		got := w.prefixToNetmask(c.pref)
		if got != c.want {
			t.Errorf("prefixToNetmask(%q) = %q; want %q", c.pref, got, c.want)
		}
	}
}

func TestPrefixToNetmask_Invalid(t *testing.T) {
	w := NewWrapper(nil)
	for _, bad := range []string{"-1", "33", "foo", ""} {
		got := w.prefixToNetmask(bad)
		if got != "255.255.255.255" {
			t.Errorf("prefixToNetmask(%q) = %q; want fallback 255.255.255.255", bad, got)
		}
	}
}

// Ensure that prefixToNetmask matches Go's CIDRMask directly.
func TestPrefixMatchesCIDRMask(t *testing.T) {
	w := NewWrapper(nil)
	for p := 0; p <= 32; p++ {
		wantBytes := net.CIDRMask(p, 32)
		want := fmt.Sprintf("%d.%d.%d.%d", wantBytes[0], wantBytes[1], wantBytes[2], wantBytes[3])
		got := w.prefixToNetmask(strconv.Itoa(p))
		if got != want {
			t.Errorf("prefixToNetmask(%d) = %q; want %q", p, got, want)
		}
	}
}
func TestSetMTU_IgnoresNonPositive(t *testing.T) {
	// When mtu <= 0 the method should be a no-op and not call commander.
	w := NewWrapper(nil)
	if err := w.SetMTU("eth0", 0); err != nil {
		t.Fatalf("expected nil error for mtu<=0, got %v", err)
	}
	if err := w.SetMTU("eth0", -1500); err != nil {
		t.Fatalf("expected nil error for mtu<=0, got %v", err)
	}
}

func TestSetMTU_CommandFails(t *testing.T) {
	fc := &fakeCommander{
		wantName: "ifconfig",
		wantArgs: []string{"eth0", "mtu", "1500"},
		out:      []byte("denied"),
		err:      errors.New("fail"),
	}
	w := NewWrapper(fc)
	err := w.SetMTU("eth0", 1500)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "ifconfig set mtu failed: fail; output: denied") {
		t.Errorf("unexpected error message: %v", err)
	}
	if !fc.called {
		t.Error("expected CombinedOutput to be called")
	}
}

func TestSetMTU_Success(t *testing.T) {
	fc := &fakeCommander{
		wantName: "ifconfig",
		wantArgs: []string{"tun0", "mtu", "9000"},
		out:      []byte(""),
		err:      nil,
	}
	w := NewWrapper(fc)
	if err := w.SetMTU("tun0", 9000); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !fc.called {
		t.Error("expected CombinedOutput to be called")
	}
}
