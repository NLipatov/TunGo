package iptables

import (
	"errors"
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
		{"Enable6DevMasquerade", func() error { return w.Enable6DevMasquerade(dev) }},
		{"Disable6DevMasquerade", func() error { return w.Disable6DevMasquerade(dev) }},
		{"Enable6ForwardingFromTunToDev", func() error { return w.Enable6ForwardingFromTunToDev(tun, dev) }},
		{"Disable6ForwardingFromTunToDev", func() error { return w.Disable6ForwardingFromTunToDev(tun, dev) }},
		{"Enable6ForwardingFromDevToTun", func() error { return w.Enable6ForwardingFromDevToTun(tun, dev) }},
		{"Disable6ForwardingFromDevToTun", func() error { return w.Disable6ForwardingFromDevToTun(tun, dev) }},
		{"Enable6ForwardingTunToTun", func() error { return w.Enable6ForwardingTunToTun(tun) }},
		{"Disable6ForwardingTunToTun", func() error { return w.Disable6ForwardingTunToTun(tun) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestWrapper_IPv6_Errors(t *testing.T) {
	const dev = "eth0"
	const tun = "tun0"

	errFail := errors.New("fail")
	failAll := &mockCommander{
		outputMap: map[string][]byte{},
		errMap:    make(map[string]error),
	}
	// Fill errMap for every possible ip6tables command so all calls fail.
	for _, cmd := range []string{
		"ip6tables -t nat -A POSTROUTING -o " + dev + " -j MASQUERADE",
		"ip6tables -t nat -D POSTROUTING -o " + dev + " -j MASQUERADE",
		"ip6tables -A FORWARD -i " + tun + " -o " + dev + " -j ACCEPT",
		"ip6tables -D FORWARD -i " + tun + " -o " + dev + " -j ACCEPT",
		"ip6tables -A FORWARD -i " + dev + " -o " + tun + " -m state --state RELATED,ESTABLISHED -j ACCEPT",
		"ip6tables -D FORWARD -i " + dev + " -o " + tun + " -m state --state RELATED,ESTABLISHED -j ACCEPT",
		"ip6tables -A FORWARD -i " + tun + " -o " + tun + " -j ACCEPT",
		"ip6tables -D FORWARD -i " + tun + " -o " + tun + " -j ACCEPT",
	} {
		failAll.errMap[cmd] = errFail
	}
	w := &Wrapper{commander: failAll}

	tests := []struct {
		name string
		call func() error
	}{
		{"Enable6DevMasquerade", func() error { return w.Enable6DevMasquerade(dev) }},
		{"Disable6DevMasquerade", func() error { return w.Disable6DevMasquerade(dev) }},
		{"Enable6ForwardingFromTunToDev", func() error { return w.Enable6ForwardingFromTunToDev(tun, dev) }},
		{"Disable6ForwardingFromTunToDev", func() error { return w.Disable6ForwardingFromTunToDev(tun, dev) }},
		{"Enable6ForwardingFromDevToTun", func() error { return w.Enable6ForwardingFromDevToTun(tun, dev) }},
		{"Disable6ForwardingFromDevToTun", func() error { return w.Disable6ForwardingFromDevToTun(tun, dev) }},
		{"Enable6ForwardingTunToTun", func() error { return w.Enable6ForwardingTunToTun(tun) }},
		{"Disable6ForwardingTunToTun", func() error { return w.Disable6ForwardingTunToTun(tun) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Error("expected error")
			}
		})
	}
}
