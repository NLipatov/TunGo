package server

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
	"tungo/infrastructure/PAL/network/linux/epoll"
	"tungo/infrastructure/PAL/network/linux/ioctl"
	"tungo/infrastructure/PAL/network/linux/ip"
	"tungo/infrastructure/PAL/network/linux/iptables"
	"tungo/infrastructure/PAL/network/linux/mssclamp"
	"tungo/infrastructure/PAL/network/linux/sysctl"
	"tungo/infrastructure/settings"
)

type TunFactory struct {
	device   tunDeviceManager
	firewall firewallConfigurator
	wrapper  tun.Wrapper
}

func NewTunFactory() tun.ServerManager {
	return &TunFactory{
		device: tunDeviceManager{
			ip:    ip.NewWrapper(exec_commander.NewExecCommander()),
			ioctl: ioctl.NewWrapper(ioctl.NewLinuxIoctlCommander(), "/dev/net/tun"),
		},
		firewall: firewallConfigurator{
			iptables: iptables.NewWrapper(exec_commander.NewExecCommander()),
			sysctl:   sysctl.NewWrapper(exec_commander.NewExecCommander()),
			mss:      mssclamp.NewManager(exec_commander.NewExecCommander()),
		},
		wrapper: epoll.NewWrapper(),
	}
}

func (s TunFactory) CreateDevice(connSettings settings.Settings) (tun.Device, error) {
	ipv4 := connSettings.IPv4Subnet.IsValid() && connSettings.IPv4Subnet.Addr().Is4()
	ipv6 := connSettings.IPv6Subnet.IsValid()

	if err := s.firewall.enableKernelForwarding(ipv4, ipv6); err != nil {
		return nil, err
	}

	tunFile, err := s.device.create(connSettings, ipv4, ipv6)
	if err != nil {
		return nil, fmt.Errorf("failed to open TUN interface: %w", err)
	}

	tunName, err := s.device.detectName(tunFile)
	if err != nil {
		_ = tunFile.Close()
		_ = s.DisposeDevices(connSettings)
		return nil, fmt.Errorf("failed to configure a server: failed to determine tunnel ifName: %w", err)
	}

	extIface, err := s.device.externalInterface()
	if err != nil {
		_ = tunFile.Close()
		_ = s.DisposeDevices(connSettings)
		return nil, fmt.Errorf("failed to configure a server: %w", err)
	}

	if configureErr := s.firewall.configure(tunName, extIface, connSettings, ipv4, ipv6); configureErr != nil {
		_ = tunFile.Close()
		if cleanupErr := s.DisposeDevices(connSettings); cleanupErr != nil {
			return nil, fmt.Errorf("failed to configure a server: %s; cleanup failed: %v", configureErr, cleanupErr)
		}
		return nil, fmt.Errorf("failed to configure a server: %s\n", configureErr)
	}

	return s.wrapper.Wrap(tunFile)
}

func (s TunFactory) DisposeDevices(connSettings settings.Settings) error {
	ifName := connSettings.TunName
	ifaceExists := true

	// If interface does not exist, continue with best-effort network cleanup:
	// stale forwarding/NAT/MSS rules can still be present after unclean shutdown.
	if _, err := net.InterfaceByName(ifName); err != nil {
		if s.isBenignInterfaceError(err) {
			ifaceExists = false
		} else {
			return fmt.Errorf("could not find interface %s: %w", ifName, err)
		}
	}

	extIface, _ := s.device.externalInterface()
	s.firewall.teardown(ifName, extIface, connSettings)

	if ifaceExists {
		if err := s.device.delete(ifName); err != nil {
			return fmt.Errorf("error deleting TUN device: %v", err)
		}
	}
	return nil
}

func (s TunFactory) Unconfigure(tunFile *os.File) error {
	tunName, err := s.device.detectName(tunFile)
	if err != nil {
		log.Printf("failed to determine tunnel ifName: %s\n", err)
	}

	extIface, err := s.device.externalInterface()
	if err != nil {
		return fmt.Errorf("failed to resolve default interface: %v", err)
	}

	// Avoid unscoped NAT cleanup here: without settings we cannot safely know
	// which source subnet rule belongs to this tunnel.
	log.Printf("skipping NAT cleanup in Unconfigure for %s: source subnet unknown, use DisposeDevices(settings)", extIface)

	return s.firewall.unconfigure(tunName, extIface)
}

func (s TunFactory) isBenignInterfaceError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.ENODEV) {
		return true
	}
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
