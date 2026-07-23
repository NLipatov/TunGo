//go:build darwin

package ifconfig

import (
	"errors"
	"fmt"
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

// --- Factory tests ---

func TestNewFactory_ReturnsNonNil(t *testing.T) {
	f := NewFactory(&mockCommander{})
	if f == nil {
		t.Fatal("expected non-nil Factory")
	}
}

func TestFactory_NewV4_ReturnsContract(t *testing.T) {
	f := NewFactory(&mockCommander{})
	c := f.NewV4()
	if c == nil {
		t.Fatal("expected non-nil Contract from NewV4")
	}
	if _, ok := c.(*v4); !ok {
		t.Fatalf("expected *v4, got %T", c)
	}
}

func TestFactory_NewV6_ReturnsContract(t *testing.T) {
	f := NewFactory(&mockCommander{})
	c := f.NewV6()
	if c == nil {
		t.Fatal("expected non-nil Contract from NewV6")
	}
	if _, ok := c.(*v6); !ok {
		t.Fatalf("expected *v6, got %T", c)
	}
}

// --- v4.LinkAddrAdd tests ---

func TestV4LinkAddrAdd_ValidCIDR(t *testing.T) {
	m := &mockCommander{}
	c := newV4(m)

	err := c.LinkAddrAdd("utun7", "10.0.0.1/24")
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
			c := newV4(m)

			if err := c.LinkAddrAdd("utun0", tt.cidr); err != nil {
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

func TestV4LinkAddrAdd_InvalidCIDR_NoSlash(t *testing.T) {
	m := &mockCommander{}
	c := newV4(m)

	err := c.LinkAddrAdd("utun0", "10.0.0.1")
	if err == nil {
		t.Fatal("expected error for CIDR without slash")
	}
	if !strings.Contains(err.Error(), "invalid CIDR") {
		t.Errorf("expected 'invalid CIDR' in error, got: %v", err)
	}
	if len(m.calls) != 0 {
		t.Errorf("expected no commander calls, got %d", len(m.calls))
	}
}

func TestV4LinkAddrAdd_NotIPv4(t *testing.T) {
	m := &mockCommander{}
	c := newV4(m)

	err := c.LinkAddrAdd("utun0", "fd00::1/64")
	if err == nil {
		t.Fatal("expected error for IPv6 address in v4 handler")
	}
	if !strings.Contains(err.Error(), "not an IPv4 CIDR") {
		t.Errorf("expected 'not an IPv4 CIDR' in error, got: %v", err)
	}
	if len(m.calls) != 0 {
		t.Errorf("expected no commander calls, got %d", len(m.calls))
	}
}

func TestV4LinkAddrAdd_InvalidPrefix(t *testing.T) {
	tests := []struct {
		name string
		cidr string
	}{
		{"prefix_33", "10.0.0.1/33"},
		{"prefix_negative", "10.0.0.1/-1"},
		{"prefix_non_numeric", "10.0.0.1/abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mockCommander{}
			c := newV4(m)

			err := c.LinkAddrAdd("utun0", tt.cidr)
			if err == nil {
				t.Fatal("expected error for invalid prefix")
			}
			if !strings.Contains(err.Error(), "invalid IPv4 prefix") {
				t.Errorf("expected 'invalid IPv4 prefix' in error, got: %v", err)
			}
			if len(m.calls) != 0 {
				t.Errorf("expected no commander calls, got %d", len(m.calls))
			}
		})
	}
}

func TestV4LinkAddrAdd_InvalidIPAddress(t *testing.T) {
	m := &mockCommander{}
	c := newV4(m)

	err := c.LinkAddrAdd("utun0", "999.999.999.999/24")
	if err == nil {
		t.Fatal("expected error for invalid IP address")
	}
	if !strings.Contains(err.Error(), "not an IPv4 CIDR") {
		t.Errorf("expected 'not an IPv4 CIDR' in error, got: %v", err)
	}
}

func TestV4LinkAddrAdd_CommanderError(t *testing.T) {
	m := &mockCommander{
		combinedOutputBytes: []byte("some output"),
		combinedOutputErr:   errors.New("ifconfig failed"),
	}
	c := newV4(m)

	err := c.LinkAddrAdd("utun0", "10.0.0.1/24")
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
	c := newV4(m)

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

func TestV4SetMTU_ZeroMTU_NoOp(t *testing.T) {
	m := &mockCommander{}
	c := newV4(m)

	err := c.SetMTU("utun0", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.calls) != 0 {
		t.Errorf("expected no commander calls for zero MTU, got %d", len(m.calls))
	}
}

func TestV4SetMTU_NegativeMTU_NoOp(t *testing.T) {
	m := &mockCommander{}
	c := newV4(m)

	err := c.SetMTU("utun0", -100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.calls) != 0 {
		t.Errorf("expected no commander calls for negative MTU, got %d", len(m.calls))
	}
}

func TestV4SetMTU_CommanderError(t *testing.T) {
	m := &mockCommander{
		combinedOutputBytes: []byte("mtu error output"),
		combinedOutputErr:   errors.New("mtu set failed"),
	}
	c := newV4(m)

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
	c := newV6(m)

	err := c.LinkAddrAdd("utun7", "fd00::1/64")
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
	c := newV6(m)

	err := c.LinkAddrAdd("utun0", "2001:db8::1/128")
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

func TestV6LinkAddrAdd_InvalidCIDR_NoSlash(t *testing.T) {
	m := &mockCommander{}
	c := newV6(m)

	err := c.LinkAddrAdd("utun0", "fd00::1")
	if err == nil {
		t.Fatal("expected error for CIDR without slash")
	}
	if !strings.Contains(err.Error(), "invalid CIDR") {
		t.Errorf("expected 'invalid CIDR' in error, got: %v", err)
	}
	if len(m.calls) != 0 {
		t.Errorf("expected no commander calls, got %d", len(m.calls))
	}
}

func TestV6LinkAddrAdd_NotIPv6(t *testing.T) {
	m := &mockCommander{}
	c := newV6(m)

	err := c.LinkAddrAdd("utun0", "10.0.0.1/24")
	if err == nil {
		t.Fatal("expected error for IPv4 address in v6 handler")
	}
	if !strings.Contains(err.Error(), "not an IPv6 CIDR") {
		t.Errorf("expected 'not an IPv6 CIDR' in error, got: %v", err)
	}
	if len(m.calls) != 0 {
		t.Errorf("expected no commander calls, got %d", len(m.calls))
	}
}

func TestV6LinkAddrAdd_InvalidPrefixClampedTo128(t *testing.T) {
	tests := []struct {
		name       string
		cidr       string
		wantPrefix string
	}{
		{"prefix_200", "fd00::1/200", "128"},
		{"prefix_negative", "fd00::1/-1", "128"},
		{"prefix_non_numeric", "fd00::1/abc", "128"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mockCommander{}
			c := newV6(m)

			err := c.LinkAddrAdd("utun0", tt.cidr)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(m.calls) != 1 {
				t.Fatalf("expected 1 call, got %d", len(m.calls))
			}
			// prefixlen is at index 4 in the args
			got := m.calls[0].args[4]
			if got != tt.wantPrefix {
				t.Errorf("expected prefix %q, got %q", tt.wantPrefix, got)
			}
		})
	}
}

func TestV6LinkAddrAdd_InvalidIPAddress(t *testing.T) {
	m := &mockCommander{}
	c := newV6(m)

	err := c.LinkAddrAdd("utun0", "not-an-ip/64")
	if err == nil {
		t.Fatal("expected error for invalid IP address")
	}
	if !strings.Contains(err.Error(), "not an IPv6 CIDR") {
		t.Errorf("expected 'not an IPv6 CIDR' in error, got: %v", err)
	}
}

func TestV6LinkAddrAdd_CommanderError(t *testing.T) {
	m := &mockCommander{
		combinedOutputBytes: []byte("v6 output"),
		combinedOutputErr:   errors.New("v6 ifconfig failed"),
	}
	c := newV6(m)

	err := c.LinkAddrAdd("utun0", "fd00::1/64")
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
	c := newV6(m)

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

func TestV6SetMTU_ZeroMTU_NoOp(t *testing.T) {
	m := &mockCommander{}
	c := newV6(m)

	err := c.SetMTU("utun0", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.calls) != 0 {
		t.Errorf("expected no commander calls for zero MTU, got %d", len(m.calls))
	}
}

func TestV6SetMTU_NegativeMTU_NoOp(t *testing.T) {
	m := &mockCommander{}
	c := newV6(m)

	err := c.SetMTU("utun0", -50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.calls) != 0 {
		t.Errorf("expected no commander calls for negative MTU, got %d", len(m.calls))
	}
}

func TestV6SetMTU_CommanderError(t *testing.T) {
	m := &mockCommander{
		combinedOutputBytes: []byte("v6 mtu err"),
		combinedOutputErr:   errors.New("mtu v6 failed"),
	}
	c := newV6(m)

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
	constructors := []struct {
		name   string
		newFn  func(*mockCommander) Contract
	}{
		{"v4", func(m *mockCommander) Contract { return newV4(m) }},
		{"v6", func(m *mockCommander) Contract { return newV6(m) }},
	}

	tests := []struct {
		mtu          int
		expectCall   bool
		expectMTUStr string
	}{
		{-1, false, ""},
		{0, false, ""},
		{1, true, "1"},
		{1500, true, "1500"},
		{9000, true, "9000"},
	}

	for _, ctor := range constructors {
		for _, tt := range tests {
			name := fmt.Sprintf("%s/mtu_%d", ctor.name, tt.mtu)
			t.Run(name, func(t *testing.T) {
				m := &mockCommander{}
				c := ctor.newFn(m)

				err := c.SetMTU("utun0", tt.mtu)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				if tt.expectCall {
					if len(m.calls) != 1 {
						t.Fatalf("expected 1 call, got %d", len(m.calls))
					}
					if m.calls[0].args[2] != tt.expectMTUStr {
						t.Errorf("expected mtu arg %q, got %q", tt.expectMTUStr, m.calls[0].args[2])
					}
				} else {
					if len(m.calls) != 0 {
						t.Errorf("expected no calls, got %d", len(m.calls))
					}
				}
			})
		}
	}
}

// --- Table-driven: v4 LinkAddrAdd error cases ---

func TestV4LinkAddrAdd_ErrorCases(t *testing.T) {
	tests := []struct {
		name      string
		cidr      string
		wantInErr string
	}{
		{"no_slash", "10.0.0.1", "invalid CIDR"},
		{"multiple_slashes", "10.0.0.1/24/extra", "invalid CIDR"},
		{"ipv6_address", "fd00::1/64", "not an IPv4 CIDR"},
		{"empty_ip", "/24", "not an IPv4 CIDR"},
		{"garbage_ip", "notanip/24", "not an IPv4 CIDR"},
		{"prefix_33", "10.0.0.1/33", "invalid IPv4 prefix"},
		{"prefix_abc", "10.0.0.1/abc", "invalid IPv4 prefix"},
		{"empty_string", "", "invalid CIDR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mockCommander{}
			c := newV4(m)

			err := c.LinkAddrAdd("utun0", tt.cidr)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantInErr) {
				t.Errorf("expected %q in error, got: %v", tt.wantInErr, err)
			}
			if len(m.calls) != 0 {
				t.Errorf("expected no commander calls, got %d", len(m.calls))
			}
		})
	}
}

// --- Table-driven: v6 LinkAddrAdd error cases ---

func TestV6LinkAddrAdd_ErrorCases(t *testing.T) {
	tests := []struct {
		name      string
		cidr      string
		wantInErr string
	}{
		{"no_slash", "fd00::1", "invalid CIDR"},
		{"multiple_slashes", "fd00::1/64/extra", "invalid CIDR"},
		{"ipv4_address", "10.0.0.1/24", "not an IPv6 CIDR"},
		{"empty_ip", "/64", "not an IPv6 CIDR"},
		{"garbage_ip", "notanip/64", "not an IPv6 CIDR"},
		{"empty_string", "", "invalid CIDR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &mockCommander{}
			c := newV6(m)

			err := c.LinkAddrAdd("utun0", tt.cidr)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantInErr) {
				t.Errorf("expected %q in error, got: %v", tt.wantInErr, err)
			}
			if len(m.calls) != 0 {
				t.Errorf("expected no commander calls, got %d", len(m.calls))
			}
		})
	}
}
