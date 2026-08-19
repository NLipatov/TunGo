package server

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"syscall"
	"tungo/internal/config/settings"
	"tungo/internal/platform/command"
	"tungo/internal/tun/internal/linux/epoll"
	"tungo/internal/tun/internal/linux/ioctl"
	"tungo/internal/tun/internal/linux/ip"
	"tungo/internal/tun/internal/linux/iptables"
	"tungo/internal/tun/internal/linux/mssclamp"
	"tungo/internal/tun/internal/linux/sysctl"
)

type tunWrapper interface {
	Wrap(*os.File) (io.ReadWriteCloser, error)
}

type Manager struct {
	device   tunDeviceManager
	firewall firewallConfigurator
	wrapper  tunWrapper
}

func NewManager() *Manager {
	return &Manager{
		device: tunDeviceManager{
			ip:    ip.NewWrapper(command.New()),
			ioctl: ioctl.NewWrapper(ioctl.NewLinuxIoctlCommander(), "/dev/net/tun"),
		},
		firewall: firewallConfigurator{
			iptables: iptables.NewWrapper(command.New()),
			sysctl:   sysctl.NewWrapper(command.New()),
			mss:      mssclamp.NewManager(command.New()),
		},
		wrapper: epoll.NewWrapper(),
	}
}

func (s Manager) OpenTunnel(connSettings settings.Settings) (io.ReadWriteCloser, error) {
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
		_ = s.CloseTunnel(connSettings)
		return nil, fmt.Errorf("failed to configure a server: failed to determine tunnel ifName: %w", err)
	}

	extIface, err := s.device.externalInterface()
	if err != nil {
		_ = tunFile.Close()
		_ = s.CloseTunnel(connSettings)
		return nil, fmt.Errorf("failed to configure a server: %w", err)
	}

	if configureErr := s.firewall.configure(tunName, extIface, connSettings, ipv4, ipv6); configureErr != nil {
		_ = tunFile.Close()
		if cleanupErr := s.CloseTunnel(connSettings); cleanupErr != nil {
			return nil, fmt.Errorf("failed to configure a server: %s; cleanup failed: %v", configureErr, cleanupErr)
		}
		return nil, fmt.Errorf("failed to configure a server: %s", configureErr)
	}

	dev, wrapErr := s.wrapper.Wrap(tunFile)
	if wrapErr != nil {
		_ = tunFile.Close()
		_ = s.CloseTunnel(connSettings)
		return nil, fmt.Errorf("failed to wrap TUN device: %w", wrapErr)
	}
	return dev, nil
}

func (s Manager) CloseTunnel(connSettings settings.Settings) error {
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

func (s Manager) Unconfigure(tunFile *os.File) error {
	tunName, err := s.device.detectName(tunFile)
	if err != nil {
		return fmt.Errorf("failed to determine tunnel ifName: %w", err)
	}

	extIface, err := s.device.externalInterface()
	if err != nil {
		return fmt.Errorf("failed to resolve default interface: %v", err)
	}

	// Avoid unscoped NAT cleanup here: without settings we cannot safely know
	// which source subnet rule belongs to this tunnel.
	slog.Warn("skipping NAT cleanup in Unconfigure: source subnet unknown", "interface", extIface)

	return s.firewall.unconfigure(tunName, extIface)
}

func (s Manager) isBenignInterfaceError(err error) bool {
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
