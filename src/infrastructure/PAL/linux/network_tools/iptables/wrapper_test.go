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
		{"ConfigureMssClamping", func() error {
			return NewWrapper(&mockCommander{
				outputMap: map[string][]byte{
					"iptables -t mangle -A FORWARD -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu":  {},
					"iptables -t mangle -A OUTPUT -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu":   {},
					"ip6tables -t mangle -A FORWARD -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu": {},
					"ip6tables -t mangle -A OUTPUT -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu":  {},
				},
				errMap: map[string]error{},
			}).ConfigureMssClamping()
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestWrapper_ConfigureMssClamping_ErrorCases(t *testing.T) {
	errorCases := []struct {
		name   string
		errMap map[string]error
		errMsg string
	}{
		{
			name: "FORWARD IPv4 error",
			errMap: map[string]error{
				"iptables -t mangle -A FORWARD -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu": errors.New("fail1"),
			},
			errMsg: "FORWARD chain",
		},
		{
			name: "OUTPUT IPv4 error",
			errMap: map[string]error{
				"iptables -t mangle -A OUTPUT -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu": errors.New("fail2"),
			},
			errMsg: "OUTPUT chain",
		},
		{
			name: "FORWARD IPv6 error",
			errMap: map[string]error{
				"ip6tables -t mangle -A FORWARD -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu": errors.New("fail3"),
			},
			errMsg: "IPv6 MSS clamping on FORWARD",
		},
		{
			name: "OUTPUT IPv6 error",
			errMap: map[string]error{
				"ip6tables -t mangle -A OUTPUT -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu": errors.New("fail4"),
			},
			errMsg: "IPv6 MSS clamping on OUTPUT",
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			out := map[string][]byte{}
			for cmd := range tc.errMap {
				out[cmd] = []byte{}
			}
			w := newWrapperWithMocks(out, tc.errMap)

			err := w.ConfigureMssClamping()
			if err == nil || !strings.Contains(err.Error(), tc.errMsg) {
				t.Errorf("expected error containing '%s', got: %v", tc.errMsg, err)
			}
		})
	}
}
