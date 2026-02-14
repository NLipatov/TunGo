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
	"tungo/infrastructure/PAL/linux/network_tools/ioctl"
	"tungo/infrastructure/PAL/linux/network_tools/ip"
	"tungo/infrastructure/PAL/linux/network_tools/iptables"
	"tungo/infrastructure/PAL/linux/network_tools/mssclamp"
	"tungo/infrastructure/PAL/linux/network_tools/sysctl"
	"tungo/infrastructure/PAL/linux/tun/epoll"
	nIp "tungo/infrastructure/network/ip"
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
	ipv6 := connSettings.IPv6Subnet.IsValid()

	forwardingErr := s.enableForwarding(ipv6)
	if forwardingErr != nil {
		return nil, forwardingErr
	}

	tunFile, err := s.createTun(connSettings)
	if err != nil {
		return nil, fmt.Errorf("failed to open TUN interface: %w", err)
	}

	configureErr := s.configure(tunFile, ipv6)
	if configureErr != nil {
		return nil, fmt.Errorf("failed to configure a server: %s\n", configureErr)
	}

	return s.wrapper.Wrap(tunFile)
}

func (s ServerTunFactory) DisposeDevices(connSettings settings.Settings) error {
	ifName := connSettings.InterfaceName

	// If interface does not exist, treat as successful no-op.
	if _, err := net.InterfaceByName(ifName); err != nil {
		if s.isBenignInterfaceError(err) {
			// nothing to delete
			return nil
		}
		// unexpected error (permissions, etc.) — surface it
		return fmt.Errorf("could not find interface %s: %w", ifName, err)
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
	} else {
		log.Printf("skipping iptables forwarding disable for %s: external interface unknown", ifName)
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

	if err := s.iptables.DisableDevMasquerade(ifName); err != nil {
		if !s.isBenignNetfilterError(err) {
			log.Printf("disabling masquerade %s: %v", ifName, err)
		}
	}

	if err := s.iptables.Disable6DevMasquerade(ifName); err != nil {
		if !s.isBenignNetfilterError(err) {
			log.Printf("disabling IPv6 masquerade %s: %v", ifName, err)
		}
	}

	if err := s.mss.Remove(ifName); err != nil {
		if !s.isBenignNetfilterError(err) {
			log.Printf("removing MSS clamping for %s: %v", ifName, err)
		}
	}

	// For LinkDelete errors — DO NOT use isBenignNetfilterError; treat as real error.
	if err := s.ip.LinkDelete(ifName); err != nil {
		return fmt.Errorf("error deleting TUN device: %v", err)
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

func (s ServerTunFactory) createTun(settings settings.Settings) (*os.File, error) {
	// delete previous tun if any exist
	_ = s.ip.LinkDelete(settings.InterfaceName)

	devErr := s.ip.TunTapAddDevTun(settings.InterfaceName)
	if devErr != nil {
		return nil, fmt.Errorf("could not create tuntap dev: %s", devErr)
	}

	upErr := s.ip.LinkSetDevUp(settings.InterfaceName)
	if upErr != nil {
		return nil, fmt.Errorf("could not set tuntap dev up: %s", upErr)
	}

	mtuErr := s.ip.LinkSetDevMTU(settings.InterfaceName, settings.MTU)
	if mtuErr != nil {
		return nil, fmt.Errorf("could not set mtu on tuntap dev: %s", mtuErr)
	}

	serverIp, serverIpErr := nIp.AllocateServerIP(settings.InterfaceSubnet)
	if serverIpErr != nil {
		return nil, fmt.Errorf("could not allocate server IP (%s): %s", serverIp, serverIpErr)
	}

	cidrServerIp, cidrServerIpErr := nIp.ToCIDR(settings.InterfaceSubnet.String(), serverIp)
	if cidrServerIpErr != nil {
		return nil, fmt.Errorf("could not conver server IP(%s) to CIDR: %s", serverIp, cidrServerIpErr)
	}
	addrAddDev := s.ip.AddrAddDev(settings.InterfaceName, cidrServerIp)
	if addrAddDev != nil {
		return nil, fmt.Errorf("failed to convert server ip to CIDR format: %s", addrAddDev)
	}

	// Assign IPv6 address if configured
	if settings.IPv6Subnet.IsValid() {
		serverIPv6, serverIPv6Err := nIp.AllocateServerIP(settings.IPv6Subnet)
		if serverIPv6Err != nil {
			return nil, fmt.Errorf("could not allocate server IPv6 (%s): %s", settings.IPv6Subnet, serverIPv6Err)
		}
		cidr6, cidr6Err := nIp.ToCIDR(settings.IPv6Subnet.String(), serverIPv6)
		if cidr6Err != nil {
			return nil, fmt.Errorf("could not convert server IPv6 (%s) to CIDR: %s", serverIPv6, cidr6Err)
		}
		if err := s.ip.AddrAddDev(settings.InterfaceName, cidr6); err != nil {
			return nil, fmt.Errorf("failed to assign IPv6 to TUN %s: %s", settings.InterfaceName, err)
		}
	}

	tunFile, tunFileErr := s.ioctl.CreateTunInterface(settings.InterfaceName)
	if tunFileErr != nil {
		return nil, fmt.Errorf("failed to open TUN interface: %v", tunFileErr)
	}

	return tunFile, nil
}

func (s ServerTunFactory) enableForwarding(ipv6 bool) error {
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

func (s ServerTunFactory) configure(tunFile *os.File, ipv6 bool) error {
	tunName, err := s.ioctl.DetectTunNameFromFd(tunFile)
	if err != nil {
		return fmt.Errorf("failed to determing tunnel ifName: %s\n", err)
	}
	if tunName == "" {
		return fmt.Errorf("failed to get TUN interface name")
	}

	externalIfName, err := s.ip.RouteDefault()
	if err != nil {
		return err
	}

	if err := s.iptables.EnableDevMasquerade(externalIfName); err != nil {
		return fmt.Errorf("failed enabling NAT: %v", err)
	}

	if ipv6 {
		if err := s.iptables.Enable6DevMasquerade(externalIfName); err != nil {
			return fmt.Errorf("failed enabling IPv6 NAT: %v", err)
		}
	}

	if err := s.setupForwarding(tunName, externalIfName, ipv6); err != nil {
		return fmt.Errorf("failed to set up forwarding: %v", err)
	}

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

	err = s.iptables.DisableDevMasquerade(tunName)
	if err != nil {
		log.Printf("failed to disbale NAT: %s\n", err)
	}

	err = s.iptables.Disable6DevMasquerade(tunName)
	if err != nil {
		log.Printf("failed to disable IPv6 NAT: %s\n", err)
	}

	defaultIfName, defaultIfNameErr := s.ip.RouteDefault()
	if defaultIfNameErr != nil {
		return fmt.Errorf("failed to resolve default interface: %v", defaultIfNameErr)
	}

	if tunName != "" {
		if err := s.mss.Remove(tunName); err != nil {
			log.Printf("failed to remove MSS clamping for %s: %v\n", tunName, err)
		}
		if err := s.clearForwarding(tunName, defaultIfName, true); err != nil {
			return err
		}
	}

	return nil
}

func (s ServerTunFactory) setupForwarding(tunName string, extIface string, ipv6 bool) error {
	if tunName == "" {
		return fmt.Errorf("failed to get TUN interface name")
	}

	// Set up iptables rules (IPv4)
	if err := s.iptables.EnableForwardingFromTunToDev(tunName, extIface); err != nil {
		return fmt.Errorf("failed to setup forwarding rule: %s", err)
	}

	if err := s.iptables.EnableForwardingFromDevToTun(tunName, extIface); err != nil {
		return fmt.Errorf("failed to setup forwarding rule: %s", err)
	}

	if err := s.iptables.EnableForwardingTunToTun(tunName); err != nil {
		return fmt.Errorf("failed to setup client-to-client forwarding rule: %s", err)
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

func (s ServerTunFactory) clearForwarding(tunName string, extIface string, ipv6 bool) error {
	if tunName == "" {
		return fmt.Errorf("failed to get TUN interface name")
	}

	if err := s.iptables.DisableForwardingFromTunToDev(tunName, extIface); err != nil {
		return fmt.Errorf("failed to execute iptables command: %s", err)
	}

	if err := s.iptables.DisableForwardingFromDevToTun(tunName, extIface); err != nil {
		return fmt.Errorf("failed to execute iptables command: %s", err)
	}

	if err := s.iptables.DisableForwardingTunToTun(tunName); err != nil {
		return fmt.Errorf("failed to execute iptables command: %s", err)
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
