package tun_server

import (
	"fmt"
	"log"
	"os"
	"strings"
	"tungo/application"
	"tungo/infrastructure/PAL"
	"tungo/infrastructure/PAL/linux/network_tools/ioctl"
	"tungo/infrastructure/PAL/linux/network_tools/ip"
	"tungo/infrastructure/PAL/linux/network_tools/sysctl"
	nip "tungo/infrastructure/network/ip"
	"tungo/infrastructure/settings"
)

type ServerTunFactory struct {
	ip        ip.Contract
	netfilter application.Netfilter
	ioctl     ioctl.Contract
	sysctl    sysctl.Contract
}

func NewServerTunFactory(netfilter application.Netfilter) application.ServerTunManager {
	return &ServerTunFactory{
		ip:        ip.NewWrapper(PAL.NewExecCommander()),
		netfilter: netfilter,
		ioctl:     ioctl.NewWrapper(ioctl.NewLinuxIoctlCommander(), "/dev/net/tun"),
		sysctl:    sysctl.NewWrapper(PAL.NewExecCommander()),
	}
}

func (s ServerTunFactory) CreateTunDevice(connSettings settings.Settings) (application.TunDevice, error) {
	forwardingErr := s.enableForwarding()
	if forwardingErr != nil {
		return nil, forwardingErr
	}

	tunFile, err := s.createTun(connSettings)
	if err != nil {
		return nil, fmt.Errorf("failed to open TUN interface: %w", err)
	}

	configureErr := s.configure(tunFile)
	if configureErr != nil {
		_ = tunFile.Close()
		s.unconfigureByTunDevName(connSettings.InterfaceName)
		_ = s.attemptToRemoveTunDevByName(connSettings.InterfaceName)
		return nil, fmt.Errorf("failed to configure a server: %w", configureErr)
	}

	return tunFile, nil
}

func (s ServerTunFactory) DisposeTunDevices(settings settings.Settings) error {
	s.unconfigureByTunDevName(settings.InterfaceName)

	if rErr := s.attemptToRemoveTunDevByName(settings.InterfaceName); rErr != nil {
		log.Printf("attemptToRemoveTunDevByName failed: %v", rErr)
	}

	return nil
}

func (s ServerTunFactory) DisableDevMasquerade() error {
	extInterface, err := s.ip.RouteDefault()
	if err != nil {
		return fmt.Errorf("failed to detect default route iface: %s", err)
	}

	return s.netfilter.DisableDevMasquerade(extInterface)
}

func (s ServerTunFactory) createTun(settings settings.Settings) (*os.File, error) {
	// delete previous tun if any exist
	_ = s.attemptToRemoveTunDevByName(settings.InterfaceName)

	devErr := s.ip.TunTapAddDevTun(settings.InterfaceName)
	if devErr != nil {
		return nil, fmt.Errorf("could not create tuntap dev: %s", devErr)
	}

	upErr := s.ip.LinkSetDevUp(settings.InterfaceName)
	if upErr != nil {
		_ = s.ip.LinkDelete(settings.InterfaceName)
		return nil, fmt.Errorf("could not set tuntap dev up: %s", upErr)
	}

	mtuErr := s.ip.LinkSetDevMTU(settings.InterfaceName, settings.MTU)
	if mtuErr != nil {
		_ = s.ip.LinkDelete(settings.InterfaceName)
		return nil, fmt.Errorf("could not set mtu on tuntap dev: %s", mtuErr)
	}

	serverIp, serverIpErr := nip.AllocateServerIp(settings.InterfaceIPCIDR)
	if serverIpErr != nil {
		_ = s.ip.LinkDelete(settings.InterfaceName)
		return nil, fmt.Errorf("could not allocate server IP (%s): %s", serverIp, serverIpErr)
	}

	cidrServerIp, cidrServerIpErr := nip.ToCIDR(settings.InterfaceIPCIDR, serverIp)
	if cidrServerIpErr != nil {
		_ = s.ip.LinkDelete(settings.InterfaceName)
		return nil, fmt.Errorf("could not conver server IP(%s) to CIDR: %s", serverIp, cidrServerIpErr)
	}
	addrAddDev := s.ip.AddrAddDev(settings.InterfaceName, cidrServerIp)
	if addrAddDev != nil {
		_ = s.ip.LinkDelete(settings.InterfaceName)
		return nil, fmt.Errorf("failed to convert server ip to CIDR format: %s", addrAddDev)
	}

	tunFile, tunFileErr := s.ioctl.CreateTunInterface(settings.InterfaceName)
	if tunFileErr != nil {
		_ = s.ip.LinkDelete(settings.InterfaceName)
		return nil, fmt.Errorf("failed to open TUN interface: %v", tunFileErr)
	}

	return tunFile, nil
}

func (s ServerTunFactory) attemptToRemoveTunDevByName(name string) error {
	ok, exErr := s.ip.LinkExists(name)
	if exErr == nil && !ok {
		return nil
	}
	if exErr != nil {
		log.Printf("link-exists check failed for %q: %v; attempting delete anyway", name, exErr)
	}

	if delErr := s.ip.LinkDelete(name); delErr != nil {
		delErrMsg := strings.ToLower(delErr.Error())
		if strings.Contains(delErrMsg, "does not exist") ||
			strings.Contains(delErrMsg, "no such device") ||
			strings.Contains(delErrMsg, "cannot find device") ||
			strings.Contains(delErrMsg, "not found") {
			return nil
		}
		return fmt.Errorf("delete %q: %w", name, delErr)
	}
	return nil
}

func (s ServerTunFactory) enableForwarding() error {
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

	return nil
}

func (s ServerTunFactory) configure(tunFile *os.File) error {
	externalIfName, err := s.ip.RouteDefault()
	if err != nil {
		return err
	}

	err = s.netfilter.EnableDevMasquerade(externalIfName)
	if err != nil {
		return fmt.Errorf("failed enabling NAT: %v", err)
	}

	err = s.setupForwarding(tunFile, externalIfName)
	if err != nil {
		return fmt.Errorf("failed to set up forwarding: %v", err)
	}

	log.Printf("server configured")
	return nil
}

func (s ServerTunFactory) unconfigureByTunDevName(name string) {
	ext, err := s.ip.RouteDefault()
	if err != nil {
		log.Printf("failed to detect default route iface: %s", err)
		return
	}
	if err := s.netfilter.DisableForwardingFromTunToDev(name, ext); err != nil {
		log.Printf("failed to disable fwd tun->dev: %s", err)
	}
	if err := s.netfilter.DisableForwardingFromDevToTun(name, ext); err != nil {
		log.Printf("failed to disable fwd dev->tun: %s", err)
	}
}

func (s ServerTunFactory) setupForwarding(tunFile *os.File, extIface string) error {
	// Get the name of the TUN interface
	tunName, err := s.ioctl.DetectTunNameFromFd(tunFile)
	if err != nil {
		return fmt.Errorf("failed to determing tunnel ifName: %s", err)
	}
	if tunName == "" {
		return fmt.Errorf("failed to get TUN interface name")
	}

	// Set up iptables rules
	err = s.netfilter.EnableForwardingFromTunToDev(tunName, extIface)
	if err != nil {
		return fmt.Errorf("failed to setup forwarding rule: %s", err)
	}

	err = s.netfilter.EnableForwardingFromDevToTun(tunName, extIface)
	if err != nil {
		return fmt.Errorf("failed to setup forwarding rule: %s", err)
	}

	return nil
}

func (s ServerTunFactory) clearForwarding(tunFile *os.File, extIface string) error {
	tunName, err := s.ioctl.DetectTunNameFromFd(tunFile)
	if err != nil {
		return fmt.Errorf("failed to determing tunnel ifName: %w", err)
	}
	if tunName == "" {
		return fmt.Errorf("failed to get TUN interface name")
	}

	err = s.netfilter.DisableForwardingFromTunToDev(tunName, extIface)
	if err != nil {
		return fmt.Errorf("failed to execute iptables command: %s", err)
	}

	err = s.netfilter.DisableForwardingFromDevToTun(tunName, extIface)
	if err != nil {
		return fmt.Errorf("failed to execute iptables command: %s", err)
	}
	return nil
}
