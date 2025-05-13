package linux

import (
	"fmt"
	"log"
	"os"
	"tungo/infrastructure/PAL/linux/ip"
	"tungo/infrastructure/PAL/linux/iptables"
	"tungo/infrastructure/PAL/linux/syscall"
	"tungo/infrastructure/PAL/linux/sysctl"
	"tungo/infrastructure/network"
	"tungo/settings"
)

func SetupServerTun(settings settings.ConnectionSettings) (*os.File, error) {
	_, _ = ip.LinkDel(settings.InterfaceName)

	name, err := UpNewTun(settings.InterfaceName)
	if err != nil {
		log.Fatalf("failed to create interface %v: %v", settings.InterfaceName, err)
	}
	fmt.Printf("created TUN interface: %v\n", name)

	serverIp, err := network.AllocateServerIp(settings.InterfaceIPCIDR)
	if err != nil {
		log.Fatalf("failed to allocate server ip %v: %v", settings.InterfaceName, err)
	}

	cidrServerIp, err := network.ToCIDR(settings.InterfaceIPCIDR, serverIp)
	if err != nil {
		log.Fatalf("failed to conver server ip to CIDR format: %s", err)
	}
	_, err = ip.LinkAddrAdd(settings.InterfaceName, cidrServerIp)
	if err != nil {
		log.Fatalf("failed to conver server ip to CIDR format: %s", err)
	}
	fmt.Printf("assigned IP %s to interface %s\n", settings.Port, settings.InterfaceName)

	setMtuErr := ip.SetMtu(settings.InterfaceName, settings.MTU)
	if setMtuErr != nil {
		log.Fatalf("failed to set MTU: %s", setMtuErr)
	}

	tunFile, err := syscall.CreateTunInterface(settings.InterfaceName)
	if err != nil {
		log.Fatalf("failed to open TUN interface: %v", err)
	}

	return tunFile, nil
}

func Configure(tunFile *os.File) error {
	externalIfName, err := ip.RouteDefault()
	if err != nil {
		return err
	}

	err = iptables.EnableMasquerade(externalIfName)
	if err != nil {
		return fmt.Errorf("failed enabling NAT: %v", err)
	}

	err = setupForwarding(tunFile, externalIfName)
	if err != nil {
		return fmt.Errorf("failed to set up forwarding: %v", err)
	}

	configureClampingErr := iptables.ConfigureMssClamping()
	if configureClampingErr != nil {
		return configureClampingErr
	}

	log.Printf("server configured\n")
	return nil
}

func Unconfigure(tunFile *os.File) {
	tunName, err := syscall.DetectTunNameFromFd(tunFile)
	if err != nil {
		log.Printf("failed to determing tunnel ifName: %s\n", err)
	}

	err = iptables.DisableMasquerade(tunName)
	if err != nil {
		log.Printf("failed to disbale NAT: %s\n", err)
	}

	err = clearForwarding(tunFile, tunName)
	if err != nil {
		log.Printf("failed to disable forwarding: %s\n", err)
	}

	log.Printf("server unconfigured\n")
}

func setupForwarding(tunFile *os.File, extIface string) error {
	// Get the name of the TUN interface
	tunName, err := syscall.DetectTunNameFromFd(tunFile)
	if err != nil {
		return fmt.Errorf("failed to determing tunnel ifName: %s\n", err)
	}
	if tunName == "" {
		return fmt.Errorf("failed to get TUN interface name")
	}

	// Set up iptables rules
	err = iptables.AcceptForwardFromTunToDev(tunName, extIface)
	if err != nil {
		return fmt.Errorf("failed to setup forwarding rule: %s", err)
	}

	err = iptables.AcceptForwardFromDevToTun(tunName, extIface)
	if err != nil {
		return fmt.Errorf("failed to setup forwarding rule: %s", err)
	}

	return nil
}

func clearForwarding(tunFile *os.File, extIface string) error {
	tunName, err := syscall.DetectTunNameFromFd(tunFile)
	if err != nil {
		return fmt.Errorf("failed to determing tunnel ifName: %s\n", err)
	}
	if tunName == "" {
		return fmt.Errorf("failed to get TUN interface name")
	}

	err = iptables.DropForwardFromTunToDev(tunName, extIface)
	if err != nil {
		return fmt.Errorf("failed to execute iptables command: %s", err)
	}

	err = iptables.DropForwardFromDevToTun(tunName, extIface)
	if err != nil {
		return fmt.Errorf("failed to execute iptables command: %s", err)
	}
	return nil
}
func UpNewTun(ifName string) (string, error) {
	err := enableIPv4Forwarding()
	if err != nil {
		return "", err
	}

	_, err = ip.LinkAdd(ifName)
	if err != nil {
		return "", err
	}

	_, err = ip.LinkSetUp(ifName)
	if err != nil {
		return "", err
	}

	return ifName, nil
}

func enableIPv4Forwarding() error {
	output, err := sysctl.NetIpv4IpForward()
	if err != nil {
		return fmt.Errorf("failed to enable IPv4 packet forwarding: %v, output: %s", err, output)
	}

	if string(output) == "net.ipv4.ip_forward = 1\n" {
		return nil
	}

	output, err = sysctl.WNetIpv4IpForward()
	if err != nil {
		return fmt.Errorf("failed to enable IPv4 packet forwarding: %v, output: %s", err, output)
	}
	return nil
}
