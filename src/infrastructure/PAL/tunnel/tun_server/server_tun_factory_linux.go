package tun_server

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"syscall"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL/exec_commander"
	"tungo/infrastructure/PAL/network/linux/ioctl"
	"tungo/infrastructure/PAL/network/linux/ip"
	"tungo/infrastructure/PAL/network/linux/iptables"
	"tungo/infrastructure/PAL/network/linux/mssclamp"
	"tungo/infrastructure/PAL/network/linux/sysctl"
	"tungo/infrastructure/PAL/network/linux/epoll"
	"tungo/infrastructure/settings"
)

type ServerTunFactory struct {
	ip       ip.Contract
	iptables iptables.Contract
	ioctl    ioctl.Contract
	sysctl   sysctl.Contract
	mss      mssclamp.Contract
	wrapper  tun.Wrapper
}

func NewServerTunFactory() tun.ServerManager {
	return &ServerTunFactory{
		ip:       ip.NewWrapper(exec_commander.NewExecCommander()),
		iptables: iptables.NewWrapper(exec_commander.NewExecCommander()),
		ioctl:    ioctl.NewWrapper(ioctl.NewLinuxIoctlCommander(), "/dev/net/tun"),
		sysctl:   sysctl.NewWrapper(exec_commander.NewExecCommander()),
		mss:      mssclamp.NewManager(exec_commander.NewExecCommander()),
		wrapper:  epoll.NewWrapper(),
	}
}

func (s ServerTunFactory) CreateDevice(connSettings settings.Settings) (tun.Device, error) {
	ipv4 := connSettings.IPv4Subnet.IsValid() && connSettings.IPv4Subnet.Addr().Is4()
	ipv6 := connSettings.IPv6Subnet.IsValid()

	forwardingErr := s.enableForwarding(ipv4, ipv6)
	if forwardingErr != nil {
		return nil, forwardingErr
	}

	tunFile, err := s.createTun(connSettings, ipv4, ipv6)
	if err != nil {
		return nil, fmt.Errorf("failed to open TUN interface: %w", err)
	}

	configureErr := s.configure(tunFile, connSettings, ipv4, ipv6)
	if configureErr != nil {
		_ = tunFile.Close()
		if cleanupErr := s.DisposeDevices(connSettings); cleanupErr != nil {
			return nil, fmt.Errorf("failed to configure a server: %s; cleanup failed: %v", configureErr, cleanupErr)
		}
		return nil, fmt.Errorf("failed to configure a server: %s\n", configureErr)
	}

	return s.wrapper.Wrap(tunFile)
}

func (s ServerTunFactory) DisposeDevices(connSettings settings.Settings) error {
	ifName := connSettings.TunName
	natV4CIDR, _ := s.masqueradeCIDR4(connSettings)
	natV6CIDR, _ := s.masqueradeCIDR6(connSettings)
	ifaceExists := true

	// If interface does not exist, continue with best-effort network cleanup:
	// stale forwarding/NAT/MSS rules can still be present after unclean shutdown.
	if _, err := net.InterfaceByName(ifName); err != nil {
		if s.isBenignInterfaceError(err) {
			ifaceExists = false
		}
		if !s.isBenignInterfaceError(err) {
			// unexpected error (permissions, etc.) — surface it
			return fmt.Errorf("could not find interface %s: %w", ifName, err)
		}
	}

	// Try to determine external interface. If unknown, skip iptables forwarding cleanup
	// because calling iptables with empty extIface leads to noisy errors.
	extIface, _ := s.ip.RouteDefault()
	if extIface != "" {
		if err := s.iptables.DisableForwardingFromTunToDev(ifName, extIface); err != nil {
			if !s.isBenignNetfilterError(err) {
				log.Printf("disabling forwarding from %s -> %s: %v", ifName, extIface, err)
			}
		}
		if err := s.iptables.DisableForwardingFromDevToTun(ifName, extIface); err != nil {
			if !s.isBenignNetfilterError(err) {
				log.Printf("disabling forwarding to %s <- %s: %v", ifName, extIface, err)
			}
		}
		if err := s.iptables.Disable6ForwardingFromTunToDev(ifName, extIface); err != nil {
			if !s.isBenignNetfilterError(err) {
				log.Printf("disabling IPv6 forwarding from %s -> %s: %v", ifName, extIface, err)
			}
		}
		if err := s.iptables.Disable6ForwardingFromDevToTun(ifName, extIface); err != nil {
			if !s.isBenignNetfilterError(err) {
				log.Printf("disabling IPv6 forwarding to %s <- %s: %v", ifName, extIface, err)
			}
		}

		if err := s.iptables.DisableDevMasquerade(extIface, natV4CIDR); err != nil {
			if !s.isBenignNetfilterError(err) {
				log.Printf("disabling masquerade %s: %v", extIface, err)
			}
		}

		if err := s.iptables.Disable6DevMasquerade(extIface, natV6CIDR); err != nil {
			if !s.isBenignNetfilterError(err) {
				log.Printf("disabling IPv6 masquerade %s: %v", extIface, err)
			}
		}
	} else {
		log.Printf("skipping iptables cleanup for %s: external interface unknown", ifName)
	}

	if err := s.iptables.DisableForwardingTunToTun(ifName); err != nil {
		if !s.isBenignNetfilterError(err) {
			log.Printf("disabling client-to-client forwarding for %s: %v", ifName, err)
		}
	}

	if err := s.iptables.Disable6ForwardingTunToTun(ifName); err != nil {
		if !s.isBenignNetfilterError(err) {
			log.Printf("disabling IPv6 client-to-client forwarding for %s: %v", ifName, err)
		}
	}

	if err := s.mss.Remove(ifName); err != nil {
		if !s.isBenignNetfilterError(err) {
			log.Printf("removing MSS clamping for %s: %v", ifName, err)
		}
	}

	if ifaceExists {
		// For LinkDelete errors — DO NOT use isBenignNetfilterError; treat as real error.
		if err := s.ip.LinkDelete(ifName); err != nil {
			return fmt.Errorf("error deleting TUN device: %v", err)
		}
	}
	return nil
}

func (s ServerTunFactory) isBenignNetfilterError(err error) bool {
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

func (s ServerTunFactory) masqueradeCIDR4(connSettings settings.Settings) (string, error) {
	if !connSettings.IPv4Subnet.IsValid() || !connSettings.IPv4Subnet.Addr().Is4() {
		return "", fmt.Errorf("no IPv4 subnet configured")
	}
	return connSettings.IPv4Subnet.Masked().String(), nil
}

func (s ServerTunFactory) masqueradeCIDR6(connSettings settings.Settings) (string, error) {
	if connSettings.IPv6Subnet.IsValid() {
		return connSettings.IPv6Subnet.Masked().String(), nil
	}
	return "", fmt.Errorf("no IPv6 subnet configured")
}

func (s ServerTunFactory) createTun(settings settings.Settings, ipv4, ipv6 bool) (tunFile *os.File, err error) {
	created := false
	defer func() {
		if err != nil && created {
			if delErr := s.ip.LinkDelete(settings.TunName); delErr != nil {
				log.Printf("failed to rollback TUN %s after create error: %v", settings.TunName, delErr)
			}
		}
	}()

	// delete previous tun if any exist
	_ = s.ip.LinkDelete(settings.TunName)

	if err = s.ip.TunTapAddDevTun(settings.TunName); err != nil {
		return nil, fmt.Errorf("could not create tuntap dev: %s", err)
	}
	created = true

	if err = s.ip.LinkSetDevUp(settings.TunName); err != nil {
		return nil, fmt.Errorf("could not set tuntap dev up: %s", err)
	}

	if err = s.ip.LinkSetDevMTU(settings.TunName, settings.MTU); err != nil {
		return nil, fmt.Errorf("could not set mtu on tuntap dev: %s", err)
	}

	hasAddress := false
	if ipv4 {
		cidr4, cidr4Err := settings.IPv4CIDR()
		if cidr4Err != nil {
			return nil, fmt.Errorf("could not derive server IPv4 CIDR: %s", cidr4Err)
		}
		if err = s.ip.AddrAddDev(settings.TunName, cidr4); err != nil {
			return nil, fmt.Errorf("failed to convert server ip to CIDR format: %s", err)
		}
		hasAddress = true
	}

	if ipv6 {
		cidr6, cidr6Err := settings.IPv6CIDR()
		if cidr6Err != nil {
			return nil, fmt.Errorf("could not derive server IPv6 CIDR: %s", cidr6Err)
		}
		if err = s.ip.AddrAddDev(settings.TunName, cidr6); err != nil {
			return nil, fmt.Errorf("failed to assign IPv6 to TUN %s: %s", settings.TunName, err)
		}
		hasAddress = true
	}

	if !hasAddress {
		return nil, fmt.Errorf("no tunnel IP configuration: both IPv4 and IPv6 are disabled")
	}

	tunFile, err = s.ioctl.CreateTunInterface(settings.TunName)
	if err != nil {
		return nil, fmt.Errorf("failed to open TUN interface: %v", err)
	}

	return tunFile, nil
}

func (s ServerTunFactory) enableForwarding(ipv4, ipv6 bool) error {
	if ipv4 {
		output, err := s.sysctl.NetIpv4IpForward()
		if err != nil {
			return fmt.Errorf("failed to enable IPv4 packet forwarding: %v, output: %s", err, output)
		}

		if string(output) != "net.ipv4.ip_forward = 1\n" {
			output, err = s.sysctl.WNetIpv4IpForward()
			if err != nil {
				return fmt.Errorf("failed to enable IPv4 packet forwarding: %v, output: %s", err, output)
			}
		}
	}

	if ipv6 {
		output6, err6 := s.sysctl.NetIpv6ConfAllForwarding()
		if err6 != nil {
			return fmt.Errorf("failed to read IPv6 forwarding state: %v, output: %s", err6, output6)
		}

		if string(output6) != "net.ipv6.conf.all.forwarding = 1\n" {
			output6, err6 = s.sysctl.WNetIpv6ConfAllForwarding()
			if err6 != nil {
				return fmt.Errorf("failed to enable IPv6 packet forwarding: %v, output: %s", err6, output6)
			}
		}
	}

	return nil
}

func (s ServerTunFactory) configure(
	tunFile *os.File,
	connSettings settings.Settings,
	ipv4, ipv6 bool,
) (err error) {
	var (
		tunName         string
		externalIfName  string
		natV4CIDR       string
		natV6CIDR       string
		natV4Enabled    bool
		natV6Enabled    bool
		forwardingReady bool
	)

	defer func() {
		if err == nil {
			return
		}

		if forwardingReady {
			if clearErr := s.clearForwarding(tunName, externalIfName, ipv4, ipv6); clearErr != nil && !s.isBenignNetfilterError(clearErr) {
				log.Printf("rollback: failed to clear forwarding for %s: %v", tunName, clearErr)
			}
		}
		if natV4Enabled {
			if disableErr := s.iptables.DisableDevMasquerade(externalIfName, natV4CIDR); disableErr != nil && !s.isBenignNetfilterError(disableErr) {
				log.Printf("rollback: failed to disable IPv4 NAT on %s (%s): %v", externalIfName, natV4CIDR, disableErr)
			}
		}
		if natV6Enabled {
			if disableErr := s.iptables.Disable6DevMasquerade(externalIfName, natV6CIDR); disableErr != nil && !s.isBenignNetfilterError(disableErr) {
				log.Printf("rollback: failed to disable IPv6 NAT on %s (%s): %v", externalIfName, natV6CIDR, disableErr)
			}
		}
	}()

	tunName, err = s.ioctl.DetectTunNameFromFd(tunFile)
	if err != nil {
		return fmt.Errorf("failed to determing tunnel ifName: %s\n", err)
	}
	if tunName == "" {
		return fmt.Errorf("failed to get TUN interface name")
	}

	externalIfName, err = s.ip.RouteDefault()
	if err != nil {
		return err
	}

	if ipv4 {
		natV4CIDR, natErr := s.masqueradeCIDR4(connSettings)
		if natErr != nil {
			return fmt.Errorf("failed to derive IPv4 NAT source subnet: %v", natErr)
		}
		if err := s.iptables.EnableDevMasquerade(externalIfName, natV4CIDR); err != nil {
			return fmt.Errorf("failed enabling NAT: %v", err)
		}
		natV4Enabled = true
	}

	if ipv6 {
		natV6CIDR, natErr := s.masqueradeCIDR6(connSettings)
		if natErr != nil {
			return fmt.Errorf("failed to derive IPv6 NAT source subnet: %v", natErr)
		}
		if err := s.iptables.Enable6DevMasquerade(externalIfName, natV6CIDR); err != nil {
			return fmt.Errorf("failed enabling IPv6 NAT: %v", err)
		}
		natV6Enabled = true
	}

	if err := s.setupForwarding(tunName, externalIfName, ipv4, ipv6); err != nil {
		return fmt.Errorf("failed to set up forwarding: %v", err)
	}
	forwardingReady = true

	if err := s.mss.Install(tunName); err != nil {
		return fmt.Errorf("failed to install MSS clamping for %s: %v", tunName, err)
	}

	log.Printf("server configured\n")
	return nil
}

func (s ServerTunFactory) Unconfigure(tunFile *os.File) error {
	tunName, err := s.ioctl.DetectTunNameFromFd(tunFile)
	if err != nil {
		log.Printf("failed to determing tunnel ifName: %s\n", err)
	}

	defaultIfName, defaultIfNameErr := s.ip.RouteDefault()
	if defaultIfNameErr != nil {
		return fmt.Errorf("failed to resolve default interface: %v", defaultIfNameErr)
	}

	// Avoid unscoped NAT cleanup here: without settings we cannot safely know
	// which source subnet rule belongs to this tunnel.
	log.Printf("skipping NAT cleanup in Unconfigure for %s: source subnet unknown, use DisposeDevices(settings)", defaultIfName)

	if tunName != "" {
		if err := s.mss.Remove(tunName); err != nil {
			log.Printf("failed to remove MSS clamping for %s: %v\n", tunName, err)
		}
		if err := s.clearForwarding(tunName, defaultIfName, true, true); err != nil {
			return err
		}
	}

	return nil
}

func (s ServerTunFactory) setupForwarding(tunName string, extIface string, ipv4, ipv6 bool) error {
	if tunName == "" {
		return fmt.Errorf("failed to get TUN interface name")
	}

	// Set up iptables rules (IPv4)
	if ipv4 {
		if err := s.iptables.EnableForwardingFromTunToDev(tunName, extIface); err != nil {
			return fmt.Errorf("failed to setup forwarding rule: %s", err)
		}

		if err := s.iptables.EnableForwardingFromDevToTun(tunName, extIface); err != nil {
			return fmt.Errorf("failed to setup forwarding rule: %s", err)
		}

		if err := s.iptables.EnableForwardingTunToTun(tunName); err != nil {
			return fmt.Errorf("failed to setup client-to-client forwarding rule: %s", err)
		}
	}

	// Set up ip6tables rules (IPv6)
	if ipv6 {
		if err := s.iptables.Enable6ForwardingFromTunToDev(tunName, extIface); err != nil {
			return fmt.Errorf("failed to setup IPv6 forwarding rule: %s", err)
		}

		if err := s.iptables.Enable6ForwardingFromDevToTun(tunName, extIface); err != nil {
			return fmt.Errorf("failed to setup IPv6 forwarding rule: %s", err)
		}

		if err := s.iptables.Enable6ForwardingTunToTun(tunName); err != nil {
			return fmt.Errorf("failed to setup IPv6 client-to-client forwarding rule: %s", err)
		}
	}

	return nil
}

func (s ServerTunFactory) clearForwarding(tunName string, extIface string, ipv4, ipv6 bool) error {
	if tunName == "" {
		return fmt.Errorf("failed to get TUN interface name")
	}

	if ipv4 {
		if err := s.iptables.DisableForwardingFromTunToDev(tunName, extIface); err != nil {
			return fmt.Errorf("failed to execute iptables command: %s", err)
		}

		if err := s.iptables.DisableForwardingFromDevToTun(tunName, extIface); err != nil {
			return fmt.Errorf("failed to execute iptables command: %s", err)
		}

		if err := s.iptables.DisableForwardingTunToTun(tunName); err != nil {
			return fmt.Errorf("failed to execute iptables command: %s", err)
		}
	}

	if ipv6 {
		if err := s.iptables.Disable6ForwardingFromTunToDev(tunName, extIface); err != nil {
			return fmt.Errorf("failed to execute ip6tables command: %s", err)
		}

		if err := s.iptables.Disable6ForwardingFromDevToTun(tunName, extIface); err != nil {
			return fmt.Errorf("failed to execute ip6tables command: %s", err)
		}

		if err := s.iptables.Disable6ForwardingTunToTun(tunName); err != nil {
			return fmt.Errorf("failed to execute ip6tables command: %s", err)
		}
	}

	return nil
}
func (s ServerTunFactory) isBenignInterfaceError(err error) bool {
	if err == nil {
		return false
	}
	// prefer errno check
	if errors.Is(err, syscall.ENODEV) {
		return true
	}
	// fallback: some environments return textual errors
	sErr := strings.ToLower(err.Error())
	if strings.Contains(sErr, "no such device") ||
		strings.Contains(sErr, "no such network interface") ||
		strings.Contains(sErr, "no such interface") ||
		strings.Contains(sErr, "does not exist") ||
		strings.Contains(sErr, "not found") {
		return true
	}
	return false
}
