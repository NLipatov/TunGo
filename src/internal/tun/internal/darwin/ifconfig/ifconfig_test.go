//go:build darwin

package ifconfig

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"testing"
)

// mockCommander records every call and returns pre-configured results.
type mockCommander struct {
	calls []mockCall

	combinedOutputBytes []byte
	combinedOutputErr   error
	outputBytes         []byte
	outputErr           error
	runErr              error
}

type mockCall struct {
	name string
	args []string
}

func (m *mockCommander) CombinedOutput(name string, args ...string) ([]byte, error) {
	m.calls = append(m.calls, mockCall{name: name, args: args})
	return m.combinedOutputBytes, m.combinedOutputErr
}

func (m *mockCommander) Output(name string, args ...string) ([]byte, error) {
	m.calls = append(m.calls, mockCall{name: name, args: args})
	return m.outputBytes, m.outputErr
}

func (m *mockCommander) Run(name string, args ...string) error {
	m.calls = append(m.calls, mockCall{name: name, args: args})
	return m.runErr
}

// --- v4.LinkAddrAdd tests ---

func TestV4LinkAddrAdd_ValidCIDR(t *testing.T) {
	m := &mockCommander{}
	c := NewV4(m)

	err := c.LinkAddrAdd("utun7", netip.MustParsePrefix("10.0.0.1/24"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(m.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(m.calls))
	}
	call := m.calls[0]
	wantName := "ifconfig"
	wantArgs := []string{"utun7", "inet", "10.0.0.1", "10.0.0.1", "netmask", "255.255.255.0", "up"}

	if call.name != wantName {
		t.Fatalf("expected command %q, got %q", wantName, call.name)
	}
	if len(call.args) != len(wantArgs) {
		t.Fatalf("expected %d args, got %d: %v", len(wantArgs), len(call.args), call.args)
	}
	for i, want := range wantArgs {
		if call.args[i] != want {
			t.Errorf("arg[%d]: expected %q, got %q", i, want, call.args[i])
		}
	}
}

func TestV4LinkAddrAdd_DifferentMasks(t *testing.T) {
	tests := []struct {
		cidr        string
		wantNetmask string
	}{
		{"192.168.1.1/32", "255.255.255.255"},
		{"192.168.1.1/16", "255.255.0.0"},
		{"192.168.1.1/0", "0.0.0.0"},
		{"172.16.0.1/8", "255.0.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.cidr, func(t *testing.T) {
			m := &mockCommander{}
			c := NewV4(m)

			if err := c.LinkAddrAdd("utun0", netip.MustParsePrefix(tt.cidr)); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(m.calls) != 1 {
				t.Fatalf("expected 1 call, got %d", len(m.calls))
			}
			// netmask is at index 5 in the args
			got := m.calls[0].args[5]
			if got != tt.wantNetmask {
				t.Errorf("netmask: expected %q, got %q", tt.wantNetmask, got)
			}
		})
	}
}

func TestV4LinkAddrAdd_NotIPv4(t *testing.T) {
	m := &mockCommander{}
	c := NewV4(m)

	err := c.LinkAddrAdd("utun0", netip.MustParsePrefix("fd00::1/64"))
	if err == nil {
		t.Fatal("expected error for IPv6 address in v4 handler")
	}
	if !strings.Contains(err.Error(), "not an IPv4 prefix") {
		t.Errorf("expected 'not an IPv4 prefix' in error, got: %v", err)
	}
	if len(m.calls) != 0 {
		t.Errorf("expected no commander calls, got %d", len(m.calls))
	}
}

func TestV4LinkAddrAdd_CommanderError(t *testing.T) {
	m := &mockCommander{
		combinedOutputBytes: []byte("some output"),
		combinedOutputErr:   errors.New("ifconfig failed"),
	}
	c := NewV4(m)

	err := c.LinkAddrAdd("utun0", netip.MustParsePrefix("10.0.0.1/24"))
	if err == nil {
		t.Fatal("expected error when commander fails")
	}
	if !strings.Contains(err.Error(), "failed to assign IPv4") {
		t.Errorf("expected 'failed to assign IPv4' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "ifconfig failed") {
		t.Errorf("expected underlying error message, got: %v", err)
	}
	if !strings.Contains(err.Error(), "some output") {
		t.Errorf("expected commander output in error, got: %v", err)
	}
}

// --- v4.SetMTU tests ---

func TestV4SetMTU_ValidMTU(t *testing.T) {
	m := &mockCommander{}
	c := NewV4(m)

	err := c.SetMTU("utun0", 1400)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(m.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(m.calls))
	}
	call := m.calls[0]
	wantName := "ifconfig"
	wantArgs := []string{"utun0", "mtu", "1400"}

	if call.name != wantName {
		t.Fatalf("expected command %q, got %q", wantName, call.name)
	}
	if len(call.args) != len(wantArgs) {
		t.Fatalf("expected %d args, got %d: %v", len(wantArgs), len(call.args), call.args)
	}
	for i, want := range wantArgs {
		if call.args[i] != want {
			t.Errorf("arg[%d]: expected %q, got %q", i, want, call.args[i])
		}
	}
}

func TestV4SetMTU_CommanderError(t *testing.T) {
	m := &mockCommander{
		combinedOutputBytes: []byte("mtu error output"),
		combinedOutputErr:   errors.New("mtu set failed"),
	}
	c := NewV4(m)

	err := c.SetMTU("utun0", 1500)
	if err == nil {
		t.Fatal("expected error when commander fails")
	}
	if !strings.Contains(err.Error(), "ifconfig set mtu failed") {
		t.Errorf("expected 'ifconfig set mtu failed' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "mtu error output") {
		t.Errorf("expected commander output in error, got: %v", err)
	}
}

// --- v6.LinkAddrAdd tests ---

func TestV6LinkAddrAdd_ValidCIDR(t *testing.T) {
	m := &mockCommander{}
	c := NewV6(m)

	err := c.LinkAddrAdd("utun7", netip.MustParsePrefix("fd00::1/64"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(m.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(m.calls))
	}
	call := m.calls[0]
	wantName := "ifconfig"
	wantArgs := []string{"utun7", "inet6", "fd00::1", "prefixlen", "64", "up"}

	if call.name != wantName {
		t.Fatalf("expected command %q, got %q", wantName, call.name)
	}
	if len(call.args) != len(wantArgs) {
		t.Fatalf("expected %d args, got %d: %v", len(wantArgs), len(call.args), call.args)
	}
	for i, want := range wantArgs {
		if call.args[i] != want {
			t.Errorf("arg[%d]: expected %q, got %q", i, want, call.args[i])
		}
	}
}

func TestV6LinkAddrAdd_FullAddress(t *testing.T) {
	m := &mockCommander{}
	c := NewV6(m)

	err := c.LinkAddrAdd("utun0", netip.MustParsePrefix("2001:db8::1/128"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(m.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(m.calls))
	}
	call := m.calls[0]
	// prefixlen should be "128"
	if call.args[3] != "prefixlen" || call.args[4] != "128" {
		t.Errorf("expected prefixlen 128, got args: %v", call.args)
	}
}

func TestV6LinkAddrAdd_NotIPv6(t *testing.T) {
	m := &mockCommander{}
	c := NewV6(m)

	err := c.LinkAddrAdd("utun0", netip.MustParsePrefix("10.0.0.1/24"))
	if err == nil {
		t.Fatal("expected error for IPv4 address in v6 handler")
	}
	if !strings.Contains(err.Error(), "not an IPv6 prefix") {
		t.Errorf("expected 'not an IPv6 prefix' in error, got: %v", err)
	}
	if len(m.calls) != 0 {
		t.Errorf("expected no commander calls, got %d", len(m.calls))
	}
}

func TestV6LinkAddrAdd_CommanderError(t *testing.T) {
	m := &mockCommander{
		combinedOutputBytes: []byte("v6 output"),
		combinedOutputErr:   errors.New("v6 ifconfig failed"),
	}
	c := NewV6(m)

	err := c.LinkAddrAdd("utun0", netip.MustParsePrefix("fd00::1/64"))
	if err == nil {
		t.Fatal("expected error when commander fails")
	}
	if !strings.Contains(err.Error(), "failed to assign IPv6") {
		t.Errorf("expected 'failed to assign IPv6' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "v6 ifconfig failed") {
		t.Errorf("expected underlying error message, got: %v", err)
	}
	if !strings.Contains(err.Error(), "v6 output") {
		t.Errorf("expected commander output in error, got: %v", err)
	}
}

// --- v6.SetMTU tests ---

func TestV6SetMTU_ValidMTU(t *testing.T) {
	m := &mockCommander{}
	c := NewV6(m)

	err := c.SetMTU("utun0", 1280)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(m.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(m.calls))
	}
	call := m.calls[0]
	wantName := "ifconfig"
	wantArgs := []string{"utun0", "mtu", "1280"}

	if call.name != wantName {
		t.Fatalf("expected command %q, got %q", wantName, call.name)
	}
	if len(call.args) != len(wantArgs) {
		t.Fatalf("expected %d args, got %d: %v", len(wantArgs), len(call.args), call.args)
	}
	for i, want := range wantArgs {
		if call.args[i] != want {
			t.Errorf("arg[%d]: expected %q, got %q", i, want, call.args[i])
		}
	}
}

func TestV6SetMTU_CommanderError(t *testing.T) {
	m := &mockCommander{
		combinedOutputBytes: []byte("v6 mtu err"),
		combinedOutputErr:   errors.New("mtu v6 failed"),
	}
	c := NewV6(m)

	err := c.SetMTU("utun0", 1500)
	if err == nil {
		t.Fatal("expected error when commander fails")
	}
	if !strings.Contains(err.Error(), "ifconfig set mtu failed") {
		t.Errorf("expected 'ifconfig set mtu failed' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "v6 mtu err") {
		t.Errorf("expected commander output in error, got: %v", err)
	}
}

// --- Table-driven: v4 and v6 SetMTU boundary values ---

func TestSetMTU_BoundaryValues(t *testing.T) {
	type mtuSetter interface {
		SetMTU(ifName string, mtu int) error
	}
	constructors := []struct {
		name  string
		newFn func(*mockCommander) mtuSetter
	}{
		{"v4", func(m *mockCommander) mtuSetter { return NewV4(m) }},
		{"v6", func(m *mockCommander) mtuSetter { return NewV6(m) }},
	}

	tests := []int{-1, 0, 1, 1500, 9000}

	for _, ctor := range constructors {
		for _, mtu := range tests {
			name := fmt.Sprintf("%s/mtu_%d", ctor.name, mtu)
			t.Run(name, func(t *testing.T) {
				m := &mockCommander{}
				c := ctor.newFn(m)

				err := c.SetMTU("utun0", mtu)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(m.calls) != 1 {
					t.Fatalf("expected 1 call, got %d", len(m.calls))
				}
				if want := fmt.Sprint(mtu); m.calls[0].args[2] != want {
					t.Errorf("expected mtu arg %q, got %q", want, m.calls[0].args[2])
				}
			})
		}
	}
}
