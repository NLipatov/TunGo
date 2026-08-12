package mssclamp

import (
	"reflect"
	"strings"
	"testing"
)

type recordingCommander struct {
	calls     []string
	outputMap map[string][]byte
	errMap    map[string]error
}

func (m *recordingCommander) record(name string, args ...string) string {
	cmd := strings.Join(append([]string{name}, args...), " ")
	m.calls = append(m.calls, cmd)
	return cmd
}

func (m *recordingCommander) CombinedOutput(name string, args ...string) ([]byte, error) {
	cmd := m.record(name, args...)
	return m.outputMap[cmd], m.errMap[cmd]
}

func (m *recordingCommander) Output(name string, args ...string) ([]byte, error) {
	cmd := m.record(name, args...)
	return m.outputMap[cmd], m.errMap[cmd]
}

func (m *recordingCommander) Run(name string, args ...string) error {
	cmd := m.record(name, args...)
	return m.errMap[cmd]
}

func TestManager_InstallAndRemove_Iptables(t *testing.T) {
	cmd := &recordingCommander{
		outputMap: map[string][]byte{
			"iptables --version": {},
		},
		errMap: map[string]error{},
	}

	m := NewManager(cmd)
	if err := m.Install("tun0"); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if err := m.Remove("tun0"); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}

	expected := []string{
		"iptables --version",
		// Install IPv4
		"iptables -t mangle -A OUTPUT -o tun0 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu",
		"iptables -t mangle -A FORWARD -o tun0 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu",
		"iptables -t mangle -A FORWARD -i tun0 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu",
		// Probe ip6tables availability
		"ip6tables -t mangle -L -n",
		// Install IPv6
		"ip6tables -t mangle -A OUTPUT -o tun0 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu",
		"ip6tables -t mangle -A FORWARD -o tun0 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu",
		"ip6tables -t mangle -A FORWARD -i tun0 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu",
		// Remove IPv4
		"iptables -t mangle -D OUTPUT -o tun0 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu",
		"iptables -t mangle -D FORWARD -o tun0 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu",
		"iptables -t mangle -D FORWARD -i tun0 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu",
		// ip6tables probe cached — no second probe
		// Remove IPv6
		"ip6tables -t mangle -D OUTPUT -o tun0 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu",
		"ip6tables -t mangle -D FORWARD -o tun0 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu",
		"ip6tables -t mangle -D FORWARD -i tun0 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu",
	}

	if !reflect.DeepEqual(expected, cmd.calls) {
		t.Fatalf("unexpected commands.\nwant: %v\n got: %v", expected, cmd.calls)
	}
}

func TestManager_InstallAndRemove_Iptables_IPv4Only(t *testing.T) {
	cmd := &recordingCommander{
		outputMap: map[string][]byte{
			"iptables --version": {},
		},
		errMap: map[string]error{
			// ip6tables probe fails — simulates ipv6.disable=1
			"ip6tables -t mangle -L -n": assertError("ip6_tables module not found"),
		},
	}

	m := NewManager(cmd)
	if err := m.Install("tun0"); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if err := m.Remove("tun0"); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}

	expected := []string{
		"iptables --version",
		// Install IPv4 only
		"iptables -t mangle -A OUTPUT -o tun0 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu",
		"iptables -t mangle -A FORWARD -o tun0 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu",
		"iptables -t mangle -A FORWARD -i tun0 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu",
		// ip6tables probe — fails, IPv6 skipped
		"ip6tables -t mangle -L -n",
		// Remove IPv4 only (ip6tables cached as unavailable)
		"iptables -t mangle -D OUTPUT -o tun0 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu",
		"iptables -t mangle -D FORWARD -o tun0 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu",
		"iptables -t mangle -D FORWARD -i tun0 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu",
	}

	if !reflect.DeepEqual(expected, cmd.calls) {
		t.Fatalf("unexpected commands.\nwant: %v\n got: %v", expected, cmd.calls)
	}
}

func TestManager_Install_IptablesErrorBubblesUp(t *testing.T) {
	cmd := &recordingCommander{
		outputMap: map[string][]byte{
			"iptables --version": {},
		},
		errMap: map[string]error{
			"iptables -t mangle -A FORWARD -o tun0 -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu": assertError("fail"),
		},
	}

	m := NewManager(cmd)
	err := m.Install("tun0")
	if err == nil || !strings.Contains(err.Error(), "FORWARD") {
		t.Fatalf("expected FORWARD error, got: %v", err)
	}
}

func TestManager_InstallAndRemove_NftFallback(t *testing.T) {
	cmd := &recordingCommander{
		outputMap: map[string][]byte{
			"nft --version": {},
		},
		errMap: map[string]error{
			"iptables --version": assertError("missing"),
		},
	}

	m := NewManager(cmd)
	if err := m.Install("tunX"); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if err := m.Remove("tunX"); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}

	expected := []string{
		"iptables --version",
		"nft --version",
		"nft delete table inet tungo_mss",
		"nft add table inet tungo_mss",
		"nft add chain inet tungo_mss tungo_mss_output { type route hook output priority mangle ; policy accept ; }",
		"nft add chain inet tungo_mss tungo_mss_forward { type filter hook forward priority mangle ; policy accept ; }",
		"nft add rule inet tungo_mss tungo_mss_output oifname tunX tcp flags syn|rst == syn tcp option maxseg size set clamp to pmtu",
		"nft add rule inet tungo_mss tungo_mss_forward oifname tunX tcp flags syn|rst == syn tcp option maxseg size set clamp to pmtu",
		"nft add rule inet tungo_mss tungo_mss_forward iifname tunX tcp flags syn|rst == syn tcp option maxseg size set clamp to pmtu",
		"nft delete table inet tungo_mss",
	}

	if !reflect.DeepEqual(expected, cmd.calls) {
		t.Fatalf("unexpected commands.\nwant: %v\n got: %v", expected, cmd.calls)
	}
}

func TestManager_DetectBackendNoneAvailable(t *testing.T) {
	cmd := &recordingCommander{
		errMap: map[string]error{
			"iptables --version": assertError("no iptables"),
			"nft --version":      assertError("no nft"),
		},
		outputMap: map[string][]byte{},
	}

	m := NewManager(cmd)
	if err := m.Install("tun0"); err == nil {
		t.Fatal("expected Install to fail when no backend is available")
	}
}

func TestManager_RemoveNftMissingTableIsBenign(t *testing.T) {
	cmd := &recordingCommander{
		outputMap: map[string][]byte{
			"nft delete table inet tungo_mss": []byte("Error: Could not delete table: No such file or directory\n"),
		},
		errMap: map[string]error{
			"nft delete table inet tungo_mss": assertError("no table"),
		},
	}

	m := NewManager(cmd)
	m.backend = backendNft

	// Delete should succeed despite missing nft table.
	if err := m.Remove("tunY"); err != nil {
		t.Fatalf("expected Remove to ignore missing nft table, got: %v", err)
	}

	if len(cmd.calls) != 1 || cmd.calls[0] != "nft delete table inet tungo_mss" {
		t.Fatalf("unexpected commands: %v", cmd.calls)
	}
}

// assertError is a helper to keep errMap literals terse.
func assertError(msg string) error { return &fakeErr{msg: msg} }

type fakeErr struct{ msg string }

func (f *fakeErr) Error() string { return f.msg }
