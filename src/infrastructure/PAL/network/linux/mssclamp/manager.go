package mssclamp

import (
	"fmt"
	"strings"
	"tungo/infrastructure/PAL/exec_commander"
)

type backend int

const (
	backendUnknown backend = iota
	backendIptables
	backendNft

	nftTable        = "tungo_mss"
	nftOutputChain  = "tungo_mss_output"
	nftForwardChain = "tungo_mss_forward"
)

// Manager installs and removes TCP MSS clamping rules bound to a TUN device.
// TCPMSS mangle rules keep ClientHello-sized packets from blackholing inside
// the UDP tunnel by advertising an MSS that fits the effective tunnel PMTU.
type Manager struct {
	commander exec_commander.Commander
	backend   backend
	ipv6      int8 // 0=unknown, 1=available, -1=unavailable
}

func NewManager(commander exec_commander.Commander) *Manager {
	return &Manager{commander: commander}
}

// Install applies MSS clamping for IPv4 and IPv6 TCP SYN packets entering or
// leaving the given TUN interface. The rules are added before packets are
// encapsulated into the UDP tunnel to avoid PMTU blackholes for TLS traffic.
func (m *Manager) Install(tunName string) error {
	backend, err := m.detectBackend()
	if err != nil {
		return err
	}

	switch backend {
	case backendIptables:
		return m.installIptables(tunName)
	case backendNft:
		return m.installNft(tunName)
	default:
		return fmt.Errorf("unsupported MSS clamping backend")
	}
}

// Remove tears down MSS clamping rules bound to the given TUN interface.
func (m *Manager) Remove(tunName string) error {
	backend, err := m.detectBackend()
	if err != nil {
		return err
	}

	switch backend {
	case backendIptables:
		return m.removeIptables(tunName)
	case backendNft:
		return m.removeNft()
	default:
		return fmt.Errorf("unsupported MSS clamping backend")
	}
}

func (m *Manager) detectBackend() (backend, error) {
	if m.backend != backendUnknown {
		return m.backend, nil
	}

	if _, err := m.commander.Output("iptables", "--version"); err == nil {
		m.backend = backendIptables
		return m.backend, nil
	}

	if _, err := m.commander.Output("nft", "--version"); err == nil {
		m.backend = backendNft
		return m.backend, nil
	}

	return backendUnknown, fmt.Errorf("neither iptables nor nftables is available for TCP MSS clamping")
}

type describedCommand struct {
	name string
	args []string
	desc string
}

func (m *Manager) run(commands []describedCommand) error {
	for _, cmd := range commands {
		output, err := m.commander.CombinedOutput(cmd.name, cmd.args...)
		if err != nil {
			return fmt.Errorf("failed to %s: %v, output: %s", cmd.desc, err, output)
		}
	}
	return nil
}

func (m *Manager) installIptables(tunName string) error {
	ipv4Rules := []describedCommand{
		{
			name: "iptables",
			args: []string{"-t", "mangle", "-A", "OUTPUT", "-o", tunName, "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
			desc: "add IPv4 OUTPUT TCPMSS clamp for " + tunName,
		},
		{
			name: "iptables",
			args: []string{"-t", "mangle", "-A", "FORWARD", "-o", tunName, "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
			desc: "add IPv4 FORWARD oif TCPMSS clamp for " + tunName,
		},
		{
			name: "iptables",
			args: []string{"-t", "mangle", "-A", "FORWARD", "-i", tunName, "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
			desc: "add IPv4 FORWARD iif TCPMSS clamp for " + tunName,
		},
	}
	if err := m.run(ipv4Rules); err != nil {
		return err
	}

	if !m.ip6tablesUsable() {
		return nil
	}

	ipv6Rules := []describedCommand{
		{
			name: "ip6tables",
			args: []string{"-t", "mangle", "-A", "OUTPUT", "-o", tunName, "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
			desc: "add IPv6 OUTPUT TCPMSS clamp for " + tunName,
		},
		{
			name: "ip6tables",
			args: []string{"-t", "mangle", "-A", "FORWARD", "-o", tunName, "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
			desc: "add IPv6 FORWARD oif TCPMSS clamp for " + tunName,
		},
		{
			name: "ip6tables",
			args: []string{"-t", "mangle", "-A", "FORWARD", "-i", tunName, "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
			desc: "add IPv6 FORWARD iif TCPMSS clamp for " + tunName,
		},
	}
	return m.run(ipv6Rules)
}

func (m *Manager) removeIptables(tunName string) error {
	ipv4Rules := []describedCommand{
		{
			name: "iptables",
			args: []string{"-t", "mangle", "-D", "OUTPUT", "-o", tunName, "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
			desc: "delete IPv4 OUTPUT TCPMSS clamp for " + tunName,
		},
		{
			name: "iptables",
			args: []string{"-t", "mangle", "-D", "FORWARD", "-o", tunName, "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
			desc: "delete IPv4 FORWARD oif TCPMSS clamp for " + tunName,
		},
		{
			name: "iptables",
			args: []string{"-t", "mangle", "-D", "FORWARD", "-i", tunName, "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
			desc: "delete IPv4 FORWARD iif TCPMSS clamp for " + tunName,
		},
	}
	if err := m.run(ipv4Rules); err != nil {
		return err
	}

	if !m.ip6tablesUsable() {
		return nil
	}

	ipv6Rules := []describedCommand{
		{
			name: "ip6tables",
			args: []string{"-t", "mangle", "-D", "OUTPUT", "-o", tunName, "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
			desc: "delete IPv6 OUTPUT TCPMSS clamp for " + tunName,
		},
		{
			name: "ip6tables",
			args: []string{"-t", "mangle", "-D", "FORWARD", "-o", tunName, "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
			desc: "delete IPv6 FORWARD oif TCPMSS clamp for " + tunName,
		},
		{
			name: "ip6tables",
			args: []string{"-t", "mangle", "-D", "FORWARD", "-i", tunName, "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
			desc: "delete IPv6 FORWARD iif TCPMSS clamp for " + tunName,
		},
	}
	return m.run(ipv6Rules)
}

// ip6tablesUsable reports whether ip6tables can manage mangle rules.
// On kernels with ipv6.disable=1 the ip6_tables module is absent and
// all ip6tables commands fail. The result is cached for the Manager lifetime.
func (m *Manager) ip6tablesUsable() bool {
	if m.ipv6 != 0 {
		return m.ipv6 > 0
	}
	_, err := m.commander.CombinedOutput("ip6tables", "-t", "mangle", "-L", "-n")
	if err == nil {
		m.ipv6 = 1
	} else {
		m.ipv6 = -1
	}
	return m.ipv6 > 0
}

func (m *Manager) installNft(tunName string) error {
	// Clean up any stale table from previous runs so we can install a fresh set.
	_, _ = m.commander.CombinedOutput("nft", "delete", "table", "inet", nftTable)

	commands := []describedCommand{
		{
			name: "nft",
			args: []string{"add", "table", "inet", nftTable},
			desc: "create nftable table for MSS clamping",
		},
		{
			name: "nft",
			args: []string{"add", "chain", "inet", nftTable, nftOutputChain, "{", "type", "route", "hook", "output", "priority", "mangle", ";", "policy", "accept", ";", "}"},
			desc: "create nftable output chain",
		},
		{
			name: "nft",
			args: []string{"add", "chain", "inet", nftTable, nftForwardChain, "{", "type", "filter", "hook", "forward", "priority", "mangle", ";", "policy", "accept", ";", "}"},
			desc: "create nftable forward chain",
		},
		{
			name: "nft",
			args: append([]string{"add", "rule", "inet", nftTable, nftOutputChain, "oifname", tunName}, nftClampRule()...),
			desc: "add nft OUTPUT TCPMSS clamp for " + tunName,
		},
		{
			name: "nft",
			args: append([]string{"add", "rule", "inet", nftTable, nftForwardChain, "oifname", tunName}, nftClampRule()...),
			desc: "add nft FORWARD oif TCPMSS clamp for " + tunName,
		},
		{
			name: "nft",
			args: append([]string{"add", "rule", "inet", nftTable, nftForwardChain, "iifname", tunName}, nftClampRule()...),
			desc: "add nft FORWARD iif TCPMSS clamp for " + tunName,
		},
	}

	return m.run(commands)
}

func (m *Manager) removeNft() error {
	output, err := m.commander.CombinedOutput("nft", "delete", "table", "inet", nftTable)
	if err != nil {
		// Treat missing tables as benign; they mean nothing is left to clean up.
		msg := strings.ToLower(err.Error() + string(output))
		if strings.Contains(msg, "no such file or directory") ||
			strings.Contains(msg, "does not exist") ||
			strings.Contains(msg, "no such table") {
			return nil
		}
		return fmt.Errorf("failed to delete nft MSS clamp table: %v, output: %s", err, output)
	}
	return nil
}

func nftClampRule() []string {
	return []string{"tcp", "flags", "syn|rst", "==", "syn", "tcp", "option", "maxseg", "size", "set", "clamp", "to", "pmtu"}
}
