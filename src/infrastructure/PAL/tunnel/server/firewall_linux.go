package server

import (
	"fmt"
	"log/slog"
	"strings"
	"tungo/infrastructure/PAL/network/linux/iptables"
	"tungo/infrastructure/PAL/network/linux/mssclamp"
	"tungo/infrastructure/PAL/network/linux/sysctl"
	"tungo/infrastructure/settings"
)

type firewallConfigurator struct {
	iptables iptables.Contract
	sysctl   sysctl.Contract
	mss      mssclamp.Contract
}

func (f firewallConfigurator) enableKernelForwarding(ipv4, ipv6 bool) error {
	if ipv4 {
		output, err := f.sysctl.NetIpv4IpForward()
		if err != nil {
			return fmt.Errorf("failed to enable IPv4 packet forwarding: %v, output: %s", err, output)
		}

		if string(output) != "net.ipv4.ip_forward = 1\n" {
			output, err = f.sysctl.WNetIpv4IpForward()
			if err != nil {
				return fmt.Errorf("failed to enable IPv4 packet forwarding: %v, output: %s", err, output)
			}
		}
	}

	if ipv6 {
		output6, err6 := f.sysctl.NetIpv6ConfAllForwarding()
		if err6 != nil {
			return fmt.Errorf("failed to read IPv6 forwarding state: %v, output: %s", err6, output6)
		}

		if string(output6) != "net.ipv6.conf.all.forwarding = 1\n" {
			output6, err6 = f.sysctl.WNetIpv6ConfAllForwarding()
			if err6 != nil {
				return fmt.Errorf("failed to enable IPv6 packet forwarding: %v, output: %s", err6, output6)
			}
		}
	}

	return nil
}

func (f firewallConfigurator) configure(
	tunName, extIface string,
	connSettings settings.Settings,
	ipv4, ipv6 bool,
) (err error) {
	var (
		natV4CIDR    string
		natV6CIDR    string
		natV4Enabled bool
		natV6Enabled bool
		fwdReady     bool
	)

	defer func() {
		if err == nil {
			return
		}

		if fwdReady {
			if clearErr := f.clearForwarding(tunName, extIface, ipv4, ipv6); clearErr != nil && !f.isBenignError(clearErr) {
				slog.Warn("rollback failed to clear forwarding", "tun_name", tunName, "err", clearErr)
			}
		}
		if natV4Enabled {
			if disableErr := f.iptables.DisableDevMasquerade(extIface, natV4CIDR); disableErr != nil && !f.isBenignError(disableErr) {
				slog.Warn("rollback failed to disable IPv4 NAT", "interface", extIface, "cidr", natV4CIDR, "err", disableErr)
			}
		}
		if natV6Enabled {
			if disableErr := f.iptables.Disable6DevMasquerade(extIface, natV6CIDR); disableErr != nil && !f.isBenignError(disableErr) {
				slog.Warn("rollback failed to disable IPv6 NAT", "interface", extIface, "cidr", natV6CIDR, "err", disableErr)
			}
		}
	}()

	if ipv4 {
		natV4CIDR, err = masqueradeCIDR4(connSettings)
		if err != nil {
			return fmt.Errorf("failed to derive IPv4 NAT source subnet: %v", err)
		}
		if err = f.iptables.EnableDevMasquerade(extIface, natV4CIDR); err != nil {
			return fmt.Errorf("failed enabling NAT: %v", err)
		}
		natV4Enabled = true
	}

	if ipv6 {
		natV6CIDR, err = masqueradeCIDR6(connSettings)
		if err != nil {
			return fmt.Errorf("failed to derive IPv6 NAT source subnet: %v", err)
		}
		if err = f.iptables.Enable6DevMasquerade(extIface, natV6CIDR); err != nil {
			return fmt.Errorf("failed enabling IPv6 NAT: %v", err)
		}
		natV6Enabled = true
	}

	if err = f.setupForwarding(tunName, extIface, ipv4, ipv6); err != nil {
		return fmt.Errorf("failed to set up forwarding: %v", err)
	}
	fwdReady = true

	if err = f.mss.Install(tunName); err != nil {
		return fmt.Errorf("failed to install MSS clamping for %s: %v", tunName, err)
	}

	slog.Info("server configured")
	return nil
}

func (f firewallConfigurator) teardown(
	tunName, extIface string,
	connSettings settings.Settings,
) {
	if extIface == "" {
		slog.Warn("skipping iptables cleanup: external interface unknown", "tun_name", tunName)
	} else {
		if err := f.iptables.DisableForwardingFromTunToDev(tunName, extIface); err != nil {
			if !f.isBenignError(err) {
				slog.Warn("failed to disable forwarding", "from", tunName, "to", extIface, "err", err)
			}
		}
		if err := f.iptables.DisableForwardingFromDevToTun(tunName, extIface); err != nil {
			if !f.isBenignError(err) {
				slog.Warn("failed to disable reverse forwarding", "to", tunName, "from", extIface, "err", err)
			}
		}
		if err := f.iptables.Disable6ForwardingFromTunToDev(tunName, extIface); err != nil {
			if !f.isBenignError(err) {
				slog.Warn("failed to disable IPv6 forwarding", "from", tunName, "to", extIface, "err", err)
			}
		}
		if err := f.iptables.Disable6ForwardingFromDevToTun(tunName, extIface); err != nil {
			if !f.isBenignError(err) {
				slog.Warn("failed to disable reverse IPv6 forwarding", "to", tunName, "from", extIface, "err", err)
			}
		}

		natV4CIDR, _ := masqueradeCIDR4(connSettings)
		if err := f.iptables.DisableDevMasquerade(extIface, natV4CIDR); err != nil {
			if !f.isBenignError(err) {
				slog.Warn("failed to disable masquerade", "interface", extIface, "err", err)
			}
		}

		natV6CIDR, _ := masqueradeCIDR6(connSettings)
		if err := f.iptables.Disable6DevMasquerade(extIface, natV6CIDR); err != nil {
			if !f.isBenignError(err) {
				slog.Warn("failed to disable IPv6 masquerade", "interface", extIface, "err", err)
			}
		}
	}

	if err := f.iptables.DisableForwardingTunToTun(tunName); err != nil {
		if !f.isBenignError(err) {
			slog.Warn("failed to disable client-to-client forwarding", "tun_name", tunName, "err", err)
		}
	}
	if err := f.iptables.Disable6ForwardingTunToTun(tunName); err != nil {
		if !f.isBenignError(err) {
			slog.Warn("failed to disable IPv6 client-to-client forwarding", "tun_name", tunName, "err", err)
		}
	}

	if err := f.mss.Remove(tunName); err != nil {
		if !f.isBenignError(err) {
			slog.Warn("removing MSS clamping failed", "tun_name", tunName, "err", err)
		}
	}
}

func (f firewallConfigurator) unconfigure(tunName, extIface string) error {
	if tunName != "" {
		if err := f.mss.Remove(tunName); err != nil {
			slog.Warn("failed to remove MSS clamping", "tun_name", tunName, "err", err)
		}
		if err := f.clearForwarding(tunName, extIface, true, true); err != nil {
			return err
		}
	}
	return nil
}

func (f firewallConfigurator) setupForwarding(tunName, extIface string, ipv4, ipv6 bool) error {
	if tunName == "" {
		return fmt.Errorf("failed to get TUN interface name")
	}

	if ipv4 {
		if err := f.iptables.EnableForwardingFromTunToDev(tunName, extIface); err != nil {
			return fmt.Errorf("failed to setup forwarding rule: %s", err)
		}
		if err := f.iptables.EnableForwardingFromDevToTun(tunName, extIface); err != nil {
			return fmt.Errorf("failed to setup forwarding rule: %s", err)
		}
		if err := f.iptables.EnableForwardingTunToTun(tunName); err != nil {
			return fmt.Errorf("failed to setup client-to-client forwarding rule: %s", err)
		}
	}

	if ipv6 {
		if err := f.iptables.Enable6ForwardingFromTunToDev(tunName, extIface); err != nil {
			return fmt.Errorf("failed to setup IPv6 forwarding rule: %s", err)
		}
		if err := f.iptables.Enable6ForwardingFromDevToTun(tunName, extIface); err != nil {
			return fmt.Errorf("failed to setup IPv6 forwarding rule: %s", err)
		}
		if err := f.iptables.Enable6ForwardingTunToTun(tunName); err != nil {
			return fmt.Errorf("failed to setup IPv6 client-to-client forwarding rule: %s", err)
		}
	}

	return nil
}

func (f firewallConfigurator) clearForwarding(tunName, extIface string, ipv4, ipv6 bool) error {
	if tunName == "" {
		return fmt.Errorf("failed to get TUN interface name")
	}

	if ipv4 {
		if err := f.iptables.DisableForwardingFromTunToDev(tunName, extIface); err != nil {
			return fmt.Errorf("failed to execute iptables command: %s", err)
		}
		if err := f.iptables.DisableForwardingFromDevToTun(tunName, extIface); err != nil {
			return fmt.Errorf("failed to execute iptables command: %s", err)
		}
		if err := f.iptables.DisableForwardingTunToTun(tunName); err != nil {
			return fmt.Errorf("failed to execute iptables command: %s", err)
		}
	}

	if ipv6 {
		if err := f.iptables.Disable6ForwardingFromTunToDev(tunName, extIface); err != nil {
			return fmt.Errorf("failed to execute ip6tables command: %s", err)
		}
		if err := f.iptables.Disable6ForwardingFromDevToTun(tunName, extIface); err != nil {
			return fmt.Errorf("failed to execute ip6tables command: %s", err)
		}
		if err := f.iptables.Disable6ForwardingTunToTun(tunName); err != nil {
			return fmt.Errorf("failed to execute ip6tables command: %s", err)
		}
	}

	return nil
}

func (f firewallConfigurator) isBenignError(err error) bool {
	if err == nil {
		return false
	}
	errString := strings.ToLower(err.Error())
	if strings.Contains(errString, "bad rule") ||
		strings.Contains(errString, "does a matching rule exist") ||
		strings.Contains(errString, "no chain") ||
		strings.Contains(errString, "no such file or directory") ||
		strings.Contains(errString, "no chain/target/match") ||
		strings.Contains(errString, "rule does not exist") ||
		strings.Contains(errString, "not found, nothing to dispose") ||
		strings.Contains(errString, "empty interface is likely to be undesired") {
		return true
	}
	return false
}

func masqueradeCIDR4(connSettings settings.Settings) (string, error) {
	if !connSettings.IPv4Subnet.IsValid() || !connSettings.IPv4Subnet.Addr().Is4() {
		return "", fmt.Errorf("no IPv4 subnet configured")
	}
	return connSettings.IPv4Subnet.Masked().String(), nil
}

func masqueradeCIDR6(connSettings settings.Settings) (string, error) {
	if connSettings.IPv6Subnet.IsValid() {
		return connSettings.IPv6Subnet.Masked().String(), nil
	}
	return "", fmt.Errorf("no IPv6 subnet configured")
}
