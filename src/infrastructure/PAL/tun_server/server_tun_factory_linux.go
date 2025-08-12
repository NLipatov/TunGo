package tun_server

import (
	"fmt"
	"log"
	"os"
	"tungo/application"
	"tungo/infrastructure/PAL"
	"tungo/infrastructure/PAL/linux/network_tools/ioctl"
	"tungo/infrastructure/PAL/linux/network_tools/ip"
	"tungo/infrastructure/PAL/linux/network_tools/iptables"
	"tungo/infrastructure/PAL/linux/network_tools/sysctl"
	ip2 "tungo/infrastructure/network/ip"
	"tungo/infrastructure/settings"
)

type ServerTunFactory struct {
	ip       ip.Contract
	iptables iptables.Contract
	ioctl    ioctl.Contract
	sysctl   sysctl.Contract
}

func NewServerTunFactory() application.ServerTunManager {
	return &ServerTunFactory{
		ip:       ip.NewWrapper(PAL.NewExecCommander()),
		iptables: iptables.NewWrapper(PAL.NewExecCommander()),
		ioctl:    ioctl.NewWrapper(ioctl.NewLinuxIoctlCommander(), "/dev/net/tun"),
		sysctl:   sysctl.NewWrapper(PAL.NewExecCommander()),
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
		return nil, fmt.Errorf("failed to configure a server: %s\n", configureErr)
	}

	return tunFile, nil
}

func (s ServerTunFactory) DisposeTunDevices(connSettings settings.Settings) error {
	tun, openErr := s.ioctl.CreateTunInterface(connSettings.InterfaceName)
	if openErr != nil {
		return fmt.Errorf("failed to open TUN interface: %w", openErr)
	}
	s.Unconfigure(tun)

	closeErr := tun.Close()
	if closeErr != nil {
		return fmt.Errorf("failed to close TUN interface: %w", closeErr)
	}

	delErr := s.ip.LinkDelete(connSettings.InterfaceName)
	if delErr != nil {
		return fmt.Errorf("error deleting TUN device: %v", delErr)
	}

	return nil
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

	serverIp, serverIpErr := ip2.AllocateServerIp(settings.InterfaceIPCIDR)
	if serverIpErr != nil {
		return nil, fmt.Errorf("could not allocate server IP (%s): %s", serverIp, serverIpErr)
	}

	cidrServerIp, cidrServerIpErr := ip2.ToCIDR(settings.InterfaceIPCIDR, serverIp)
	if cidrServerIpErr != nil {
		return nil, fmt.Errorf("could not conver server IP(%s) to CIDR: %s", serverIp, cidrServerIpErr)
	}
	addrAddDev := s.ip.AddrAddDev(settings.InterfaceName, cidrServerIp)
	if addrAddDev != nil {
		return nil, fmt.Errorf("failed to convert server ip to CIDR format: %s", addrAddDev)
	}

	tunFile, tunFileErr := s.ioctl.CreateTunInterface(settings.InterfaceName)
	if tunFileErr != nil {
		return nil, fmt.Errorf("failed to open TUN interface: %v", tunFileErr)
	}

	return tunFile, nil
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

	err = s.iptables.EnableDevMasquerade(externalIfName)
	if err != nil {
		return fmt.Errorf("failed enabling NAT: %v", err)
	}

	err = s.setupForwarding(tunFile, externalIfName)
	if err != nil {
		return fmt.Errorf("failed to set up forwarding: %v", err)
	}

	configureClampingErr := s.iptables.ConfigureMssClamping()
	if configureClampingErr != nil {
		return configureClampingErr
	}

	log.Printf("server configured\n")
	return nil
}

func (s ServerTunFactory) Unconfigure(tunFile *os.File) {
	tunName, err := s.ioctl.DetectTunNameFromFd(tunFile)
	if err != nil {
		log.Printf("failed to determing tunnel ifName: %s\n", err)
	}

	err = s.iptables.DisableDevMasquerade(tunName)
	if err != nil {
		log.Printf("failed to disbale NAT: %s\n", err)
	}

	err = s.clearForwarding(tunFile, tunName)
	if err != nil {
		log.Printf("failed to disable forwarding: %s\n", err)
	}

	log.Printf("server unconfigured\n")
}

func (s ServerTunFactory) setupForwarding(tunFile *os.File, extIface string) error {
	// Get the name of the TUN interface
	tunName, err := s.ioctl.DetectTunNameFromFd(tunFile)
	if err != nil {
		return fmt.Errorf("failed to determing tunnel ifName: %s\n", err)
	}
	if tunName == "" {
		return fmt.Errorf("failed to get TUN interface name")
	}

	// Set up iptables rules
	err = s.iptables.EnableForwardingFromTunToDev(tunName, extIface)
	if err != nil {
		return fmt.Errorf("failed to setup forwarding rule: %s", err)
	}

	err = s.iptables.EnableForwardingFromDevToTun(tunName, extIface)
	if err != nil {
		return fmt.Errorf("failed to setup forwarding rule: %s", err)
	}

	return nil
}

func (s ServerTunFactory) clearForwarding(tunFile *os.File, extIface string) error {
	tunName, err := s.ioctl.DetectTunNameFromFd(tunFile)
	if err != nil {
		return fmt.Errorf("failed to determing tunnel ifName: %s\n", err)
	}
	if tunName == "" {
		return fmt.Errorf("failed to get TUN interface name")
	}

	err = s.iptables.DisableForwardingFromTunToDev(tunName, extIface)
	if err != nil {
		return fmt.Errorf("failed to execute iptables command: %s", err)
	}

	err = s.iptables.DisableForwardingFromDevToTun(tunName, extIface)
	if err != nil {
		return fmt.Errorf("failed to execute iptables command: %s", err)
	}
	return nil
}
