package mssclamp

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"tungo/internal/platform/command"
)

type backend int

const (
	backendUnknown backend = iota
	backendIptables
	backendNft

	legacyNftTable  = "tungo_mss"
	nftTablePrefix  = "tungo_mss_"
	nftOutputChain  = "tungo_mss_output"
	nftForwardChain = "tungo_mss_forward"
)

// Manager installs and removes TCP MSS clamping rules bound to a TUN device.
// TCPMSS mangle rules keep ClientHello-sized packets from blackholing inside
// the UDP tunnel by advertising an MSS that fits the effective tunnel PMTU.
type Manager struct {
	commander command.Runner
}

func NewManager(commander command.Runner) *Manager {
	return &Manager{commander: commander}
}

// Install applies MSS clamping for the configured IP families.
func (m *Manager) Install(tunName string, families Families) error {
	backend, err := m.detectBackend(families)
	if err != nil {
		return err
	}

	switch backend {
	case backendIptables:
		return m.installIptables(tunName, families)
	case backendNft:
		return m.installNft(tunName)
	default:
		return fmt.Errorf("unsupported MSS clamping backend")
	}
}

// Remove tears down every MSS clamping rule TunGo may own for the interface.
// It does not depend on the current configuration so stale rules can be
// removed after a crash or an IP-family change.
func (m *Manager) Remove(tunName string) error {
	var (
		cleanupErrs []error
		available   bool
	)
	if m.iptablesUsable() {
		available = true
		cleanupErrs = append(cleanupErrs, m.runCleanup(ipv4Rules(tunName, "-D", "delete")))
	}
	if m.ip6tablesUsable() {
		available = true
		cleanupErrs = append(cleanupErrs, m.runCleanup(ipv6Rules(tunName, "-D", "delete")))
	}
	if m.nftUsable() {
		available = true
		cleanupErrs = append(cleanupErrs, m.removeNft(tunName))
	}
	if !available {
		return fmt.Errorf("neither iptables nor nftables is available for TCP MSS clamping cleanup")
	}
	return errors.Join(cleanupErrs...)
}

func (m *Manager) detectBackend(families Families) (backend, error) {
	if !families.IPv4 && !families.IPv6 {
		return backendUnknown, fmt.Errorf("no IP families configured for TCP MSS clamping")
	}
	iptablesReady := !families.IPv4 || m.iptablesUsable()
	if iptablesReady && families.IPv6 {
		iptablesReady = m.ip6tablesUsable()
	}
	if iptablesReady {
		return backendIptables, nil
	}
	if m.nftUsable() {
		return backendNft, nil
	}
	return backendUnknown, fmt.Errorf("neither iptables nor nftables can configure TCP MSS clamping for the requested IP families")
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

func (m *Manager) runCleanup(commands []describedCommand) error {
	var cleanupErrs []error
	for _, cmd := range commands {
		output, err := m.commander.CombinedOutput(cmd.name, cmd.args...)
		if err != nil && !ruleMissing(output, err) {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("failed to %s: %v, output: %s", cmd.desc, err, output))
		}
	}
	return errors.Join(cleanupErrs...)
}

func (m *Manager) installIptables(tunName string, families Families) error {
	var rules []describedCommand
	if families.IPv4 {
		rules = append(rules, ipv4Rules(tunName, "-A", "add")...)
	}
	if families.IPv6 {
		rules = append(rules, ipv6Rules(tunName, "-A", "add")...)
	}
	return m.run(rules)
}

func ipv4Rules(tunName, operation, action string) []describedCommand {
	return []describedCommand{
		{
			name: "iptables",
			args: []string{"-t", "mangle", operation, "OUTPUT", "-o", tunName, "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
			desc: action + " IPv4 OUTPUT TCPMSS clamp for " + tunName,
		},
		{
			name: "iptables",
			args: []string{"-t", "mangle", operation, "FORWARD", "-o", tunName, "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
			desc: action + " IPv4 FORWARD oif TCPMSS clamp for " + tunName,
		},
		{
			name: "iptables",
			args: []string{"-t", "mangle", operation, "FORWARD", "-i", tunName, "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
			desc: action + " IPv4 FORWARD iif TCPMSS clamp for " + tunName,
		},
	}
}

func ipv6Rules(tunName, operation, action string) []describedCommand {
	return []describedCommand{
		{
			name: "ip6tables",
			args: []string{"-t", "mangle", operation, "OUTPUT", "-o", tunName, "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
			desc: action + " IPv6 OUTPUT TCPMSS clamp for " + tunName,
		},
		{
			name: "ip6tables",
			args: []string{"-t", "mangle", operation, "FORWARD", "-o", tunName, "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
			desc: action + " IPv6 FORWARD oif TCPMSS clamp for " + tunName,
		},
		{
			name: "ip6tables",
			args: []string{"-t", "mangle", operation, "FORWARD", "-i", tunName, "-p", "tcp", "--tcp-flags", "SYN,RST", "SYN", "-j", "TCPMSS", "--clamp-mss-to-pmtu"},
			desc: action + " IPv6 FORWARD iif TCPMSS clamp for " + tunName,
		},
	}
}

func (m *Manager) iptablesUsable() bool {
	_, err := m.commander.Output("iptables", "--version")
	return err == nil
}

func (m *Manager) ip6tablesUsable() bool {
	_, err := m.commander.CombinedOutput("ip6tables", "-t", "mangle", "-L", "-n")
	return err == nil
}

func (m *Manager) nftUsable() bool {
	_, err := m.commander.Output("nft", "--version")
	return err == nil
}

func (m *Manager) installNft(tunName string) error {
	table := nftTableName(tunName)
	// Clean up stale rules for this TUN and the table used by older versions.
	_, _ = m.commander.CombinedOutput("nft", "delete", "table", "inet", table)
	_, _ = m.commander.CombinedOutput("nft", "delete", "table", "inet", legacyNftTable)

	commands := []describedCommand{
		{
			name: "nft",
			args: []string{"add", "table", "inet", table},
			desc: "create nftable table for MSS clamping",
		},
		{
			name: "nft",
			args: []string{"add", "chain", "inet", table, nftOutputChain, "{", "type", "route", "hook", "output", "priority", "mangle", ";", "policy", "accept", ";", "}"},
			desc: "create nftable output chain",
		},
		{
			name: "nft",
			args: []string{"add", "chain", "inet", table, nftForwardChain, "{", "type", "filter", "hook", "forward", "priority", "mangle", ";", "policy", "accept", ";", "}"},
			desc: "create nftable forward chain",
		},
		{
			name: "nft",
			args: append([]string{"add", "rule", "inet", table, nftOutputChain, "oifname", tunName}, nftClampRule()...),
			desc: "add nft OUTPUT TCPMSS clamp for " + tunName,
		},
		{
			name: "nft",
			args: append([]string{"add", "rule", "inet", table, nftForwardChain, "oifname", tunName}, nftClampRule()...),
			desc: "add nft FORWARD oif TCPMSS clamp for " + tunName,
		},
		{
			name: "nft",
			args: append([]string{"add", "rule", "inet", table, nftForwardChain, "iifname", tunName}, nftClampRule()...),
			desc: "add nft FORWARD iif TCPMSS clamp for " + tunName,
		},
	}

	return m.run(commands)
}

func (m *Manager) removeNft(tunName string) error {
	return errors.Join(
		m.deleteNftTable(nftTableName(tunName)),
		m.deleteNftTable(legacyNftTable),
	)
}

func (m *Manager) deleteNftTable(table string) error {
	output, err := m.commander.CombinedOutput("nft", "delete", "table", "inet", table)
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

func nftTableName(tunName string) string {
	return nftTablePrefix + hex.EncodeToString([]byte(tunName))
}

func nftClampRule() []string {
	return []string{"tcp", "flags", "syn", "/", "syn,rst", "tcp", "option", "maxseg", "size", "set", "rt", "mtu"}
}

func ruleMissing(output []byte, err error) bool {
	message := strings.ToLower(err.Error() + " " + string(output))
	return strings.Contains(message, "bad rule") ||
		strings.Contains(message, "does a matching rule exist") ||
		strings.Contains(message, "no chain/target/match") ||
		strings.Contains(message, "rule does not exist")
}
