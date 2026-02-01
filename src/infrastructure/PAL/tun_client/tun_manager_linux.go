package tun_client

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/exec_commander"
	"tungo/infrastructure/PAL/linux/network_tools/ioctl"
	"tungo/infrastructure/PAL/linux/network_tools/ip"
	"tungo/infrastructure/PAL/linux/network_tools/mssclamp"
	"tungo/infrastructure/PAL/linux/tun/epoll"
	"tungo/infrastructure/settings"
)

// PlatformTunManager Linux-specific TunDevice manager
type PlatformTunManager struct {
	configuration client.Configuration
	ip            ip.Contract
	ioctl         ioctl.Contract
	mss           mssclamp.Contract
	wrapper       tun.Wrapper
}

func NewPlatformTunManager(
	configuration client.Configuration,
) (tun.ClientManager, error) {
	return &PlatformTunManager{
		configuration: configuration,
		ip:            ip.NewWrapper(exec_commander.NewExecCommander()),
		ioctl:         ioctl.NewWrapper(ioctl.NewLinuxIoctlCommander(), "/dev/net/tun"),
		mss:           mssclamp.NewManager(exec_commander.NewExecCommander()),
		wrapper:       epoll.NewWrapper(),
	}, nil
}

func (t *PlatformTunManager) CreateDevice() (tun.Device, error) {
	connectionSettings, connectionSettingsErr := t.configuration.ActiveSettings()
	if connectionSettingsErr != nil {
		return nil, connectionSettingsErr
	}

	// configureTUN client
	if udpConfigurationErr := t.configureTUN(connectionSettings); udpConfigurationErr != nil {
		return nil, fmt.Errorf("failed to configure client: %v", udpConfigurationErr)
	}

	// opens the TUN device
	tunFile, openTunErr := t.ioctl.CreateTunInterface(connectionSettings.InterfaceName)
	if openTunErr != nil {
		return nil, fmt.Errorf("failed to open TUN interface: %v", openTunErr)
	}

	return t.wrapper.Wrap(tunFile)
}

// configureTUN Configures client's TUN device (creates the TUN device, assigns an IP to it, etc)
func (t *PlatformTunManager) configureTUN(connSettings settings.Settings) error {
	err := t.ip.TunTapAddDevTun(connSettings.InterfaceName)
	if err != nil {
		return err
	}

	err = t.ip.LinkSetDevUp(connSettings.InterfaceName)
	if err != nil {
		return err
	}
	fmt.Printf("created TUN interface: %v\n", connSettings.InterfaceName)

	// Assign IP address to the TUN interface
	err = t.ip.AddrAddDev(connSettings.InterfaceName, connSettings.InterfaceAddress)
	if err != nil {
		return err
	}
	fmt.Printf("assigned IP %s to interface %s\n", connSettings.InterfaceAddress, connSettings.InterfaceName)

	// Parse server IP
	serverIP := connSettings.ConnectionIP

	// Get routing information
	routeInfo, err := t.ip.RouteGet(serverIP)
	if err != nil {
		return err
	}
	var viaGateway, devInterface string
	fields := strings.Fields(routeInfo)
	for i, field := range fields {
		if field == "via" && i+1 < len(fields) {
			viaGateway = fields[i+1]
		}
		if field == "dev" && i+1 < len(fields) {
			devInterface = fields[i+1]
		}
	}
	if devInterface == "" {
		return fmt.Errorf("failed to parse route to server IP")
	}

	// Add route to server IP
	if viaGateway == "" {
		err = t.ip.RouteAddDev(serverIP, devInterface)
	} else {
		err = t.ip.RouteAddViaDev(serverIP, devInterface, viaGateway)
	}
	if err != nil {
		return fmt.Errorf("failed to add route to server IP: %v", err)
	}
	fmt.Printf("added route to server %s via %s dev %s\n", serverIP, viaGateway, devInterface)

	// Set the TUN interface as the default gateway
	err = t.ip.RouteAddDefaultDev(connSettings.InterfaceName)
	if err != nil {
		return err
	}
	fmt.Printf("set %s as default gateway\n", connSettings.InterfaceName)

	// sets client's TUN device maximum transmission unit (MTU)
	if setMtuErr := t.ip.LinkSetDevMTU(connSettings.InterfaceName, connSettings.MTU); setMtuErr != nil {
		return fmt.Errorf(
			"failed to set %d MTU for %s: %s", connSettings.MTU, connSettings.InterfaceName, setMtuErr,
		)
	}

	// install MSS clamping to prevent PMTU blackholes when forwarding traffic
	if err := t.mss.Install(connSettings.InterfaceName); err != nil {
		return fmt.Errorf("failed to install MSS clamping for %s: %v", connSettings.InterfaceName, err)
	}

	return nil
}

func (t *PlatformTunManager) DisposeDevices() error {
	var errs []error

	for _, s := range []struct{ iface, connIP string }{
		{t.configuration.UDPSettings.InterfaceName, t.configuration.UDPSettings.ConnectionIP},
		{t.configuration.TCPSettings.InterfaceName, t.configuration.TCPSettings.ConnectionIP},
		{t.configuration.WSSettings.InterfaceName, t.configuration.WSSettings.ConnectionIP},
	} {
		if err := t.mss.Remove(s.iface); err != nil {
			log.Printf("cleanup: failed to remove MSS clamping for %s: %v", s.iface, err)
		}
		if err := t.ip.RouteDel(s.connIP); err != nil && !isBenignCleanupError(err) {
			log.Printf("cleanup: failed to delete route for %s: %v", s.connIP, err)
			errs = append(errs, fmt.Errorf("route del %s: %w", s.connIP, err))
		}
		if err := t.ip.LinkDelete(s.iface); err != nil && !isBenignCleanupError(err) {
			log.Printf("cleanup: failed to delete link %s: %v", s.iface, err)
			errs = append(errs, fmt.Errorf("link delete %s: %w", s.iface, err))
		}
	}

	return errors.Join(errs...)
}

// isBenignCleanupError returns true when the error indicates the resource is
// already absent. Deleting something that doesn't exist is idempotent success.
func isBenignCleanupError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such process") ||
		strings.Contains(msg, "cannot find device") ||
		strings.Contains(msg, "no such device") ||
		strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "not found")
}
