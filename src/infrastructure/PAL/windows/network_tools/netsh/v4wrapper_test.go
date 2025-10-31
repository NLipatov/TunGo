//go:build windows

package netsh

import (
	"errors"
	"testing"
)

type mockCommander struct {
	name   string
	args   []string
	output []byte
	err    error
}

func (m *mockCommander) CombinedOutput(name string, args ...string) ([]byte, error) {
	m.name = name
	m.args = args
	return m.output, m.err
}
func (m *mockCommander) Output(string, ...string) ([]byte, error) { return nil, nil }
func (m *mockCommander) Run(string, ...string) error              { return nil }

func TestNetshWrapper_AllMethods(t *testing.T) {
	tests := []struct {
		name     string
		call     func(w *V4Wrapper) error
		wantCmd  string
		wantArgs []string
	}{
		{
			name:    "DeleteDefaultRoute",
			call:    func(w *V4Wrapper) error { return w.DeleteDefaultRoute("Ethernet 1") },
			wantCmd: "netsh",
		},
		{
			name:    "DeleteAddress",
			call:    func(w *V4Wrapper) error { return w.DeleteAddress("Ethernet 1", "192.168.1.2") },
			wantCmd: "netsh",
		},
		{
			name:    "SetMTU",
			call:    func(w *V4Wrapper) error { return w.SetMTU("Ethernet 1", 1500) },
			wantCmd: "netsh",
		},
		{
			name:    "AddRoutePrefix",
			call:    func(w *V4Wrapper) error { return w.AddRoutePrefix("10.0.0.0/24", "Ethernet 1", 10) },
			wantCmd: "netsh",
		},
		{
			name:    "DeleteRoutePrefix",
			call:    func(w *V4Wrapper) error { return w.DeleteRoutePrefix("10.0.0.0/24", "Ethernet 1") },
			wantCmd: "netsh",
		},
		{
			name: "SetAddressStatic",
			call: func(w *V4Wrapper) error {
				return w.SetAddressStatic("Ethernet 1", "10.0.0.2", "255.255.255.0")
			},
			wantCmd: "netsh",
		},
		{
			name: "SetAddressWithGateway",
			call: func(w *V4Wrapper) error {
				return w.SetAddressWithGateway("Ethernet 1", "10.0.0.2", "255.255.255.0", "10.0.0.1", 5)
			},
			wantCmd: "netsh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+"_success", func(t *testing.T) {
			mock := &mockCommander{output: []byte("OK"), err: nil}
			w := NewV4Wrapper(mock)

			if err := tt.call(w.(*V4Wrapper)); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mock.name != tt.wantCmd {
				t.Errorf("expected command %q, got %q", tt.wantCmd, mock.name)
			}
		})
		t.Run(tt.name+"_error", func(t *testing.T) {
			mock := &mockCommander{output: []byte("failure"), err: errors.New("exit 1")}
			w := NewV4Wrapper(mock)

			err := tt.call(w.(*V4Wrapper))
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			want := "output: failure"
			if got := err.Error(); !contains(got, want) {
				t.Errorf("expected %q in error, got %q", want, got)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (contains(s[1:], substr) || contains(s[:len(s)-1], substr)))
}
