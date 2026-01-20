package iptables

import (
	"strings"
	"testing"
)

type mockCommander struct {
	outputMap map[string][]byte
	errMap    map[string]error
}

func (m *mockCommander) Run(_ string, _ ...string) error {
	panic("not implemented")
}

func (m *mockCommander) CombinedOutput(name string, args ...string) ([]byte, error) {
	cmd := strings.Join(append([]string{name}, args...), " ")
	return m.outputMap[cmd], m.errMap[cmd]
}

func (m *mockCommander) Output(name string, args ...string) ([]byte, error) {
	cmd := strings.Join(append([]string{name}, args...), " ")
	return m.outputMap[cmd], m.errMap[cmd]
}

func newWrapperWithMocks(out map[string][]byte, errs map[string]error) *Wrapper {
	return NewWrapper(&mockCommander{
		outputMap: out,
		errMap:    errs,
	})
}

func TestWrapper_AllCommands(t *testing.T) {
	const dev = "eth0"
	const tun = "tun0"

	successOut := map[string][]byte{}
	noErr := map[string]error{}

	w := newWrapperWithMocks(successOut, noErr)

	tests := []struct {
		name string
		call func() error
	}{
		{"EnableDevMasquerade", func() error { return w.EnableDevMasquerade(dev) }},
		{"DisableDevMasquerade", func() error { return w.DisableDevMasquerade(dev) }},
		{"EnableForwardingFromTunToDev", func() error { return w.EnableForwardingFromTunToDev(tun, dev) }},
		{"DisableForwardingFromTunToDev", func() error { return w.DisableForwardingFromTunToDev(tun, dev) }},
		{"EnableForwardingFromDevToTun", func() error { return w.EnableForwardingFromDevToTun(tun, dev) }},
		{"DisableForwardingFromDevToTun", func() error { return w.DisableForwardingFromDevToTun(tun, dev) }},
		{"EnableForwardingTunToTun", func() error { return w.EnableForwardingTunToTun(tun) }},
		{"DisableForwardingTunToTun", func() error { return w.DisableForwardingTunToTun(tun) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
