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
		call     func(w *Wrapper) error
		wantCmd  string
		wantArgs []string
	}{
		{
			name:    "RouteDelete",
			call:    func(w *Wrapper) error { return w.RouteDelete("10.0.0.1") },
			wantCmd: "route",
		},
		{
			name:    "InterfaceDeleteDefaultRoute",
			call:    func(w *Wrapper) error { return w.InterfaceDeleteDefaultRoute("Ethernet 1") },
			wantCmd: "netsh",
		},
		{
			name:    "InterfaceIPDeleteAddress",
			call:    func(w *Wrapper) error { return w.InterfaceIPDeleteAddress("Ethernet 1", "192.168.1.2") },
			wantCmd: "netsh",
		},
		{
			name:    "SetInterfaceMetric",
			call:    func(w *Wrapper) error { return w.SetInterfaceMetric("Ethernet 1", 25) },
			wantCmd: "netsh",
		},
		{
			name:    "LinkSetDevMTU",
			call:    func(w *Wrapper) error { return w.LinkSetDevMTU("Ethernet 1", 1500) },
			wantCmd: "netsh",
		},
		{
			name:    "InterfaceAddRouteOnLink",
			call:    func(w *Wrapper) error { return w.InterfaceAddRouteOnLink("10.0.0.0/24", "Ethernet 1", 10) },
			wantCmd: "netsh",
		},
		{
			name:    "InterfaceDeleteRoute",
			call:    func(w *Wrapper) error { return w.InterfaceDeleteRoute("10.0.0.0/24", "Ethernet 1") },
			wantCmd: "netsh",
		},
		{
			name: "InterfaceSetAddressNoGateway",
			call: func(w *Wrapper) error {
				return w.InterfaceSetAddressNoGateway("Ethernet 1", "10.0.0.2", "255.255.255.0")
			},
			wantCmd: "netsh",
		},
		{
			name: "InterfaceSetAddressWithGateway",
			call: func(w *Wrapper) error {
				return w.InterfaceSetAddressWithGateway("Ethernet 1", "10.0.0.2", "255.255.255.0", "10.0.0.1", 5)
			},
			wantCmd: "netsh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+"_success", func(t *testing.T) {
			mock := &mockCommander{output: []byte("OK"), err: nil}
			w := NewWrapper(mock)

			if err := tt.call(w.(*Wrapper)); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mock.name != tt.wantCmd {
				t.Errorf("expected command %q, got %q", tt.wantCmd, mock.name)
			}
		})
		t.Run(tt.name+"_error", func(t *testing.T) {
			mock := &mockCommander{output: []byte("failure"), err: errors.New("exit 1")}
			w := NewWrapper(mock)

			err := tt.call(w.(*Wrapper))
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
