//go:build windows

package manager

import (
	"fmt"
	"io"
	"net/netip"
	"tungo/internal/tun/internal/windows/ipcfg"

	"tungo/internal/config/settings"
)

type clientManager interface {
	OpenTunnel(serverAddr netip.Addr) (io.ReadWriteCloser, error)
	CloseTunnel() error
}

type networkConfigurator interface {
	FlushDNS() error
	SetAddressStatic(ifName string, prefix netip.Prefix) error
	SetDNS(ifName string, dnsServers []string) error
	SetMTU(ifName string, mtu int) error
	AddHostRouteViaGateway(hostIP netip.Addr, ifName string, gateway netip.Addr, metric int) error
	AddHostRouteOnLink(hostIP netip.Addr, ifName string, metric int) error
	AddDefaultSplitRoutes(ifName string, metric int) error
	DeleteDefaultSplitRoutes(ifName string) error
	DeleteRoute(destination netip.Addr) error
	DeleteRouteOnInterface(destination netip.Addr, ifName string) error
	BestRoute(dest netip.Addr) (netip.Addr, string, int, int, error)
}

func New(connectionSettings settings.Settings) (clientManager, error) {
	has4 := connectionSettings.IPv4.IsValid() && !connectionSettings.IPv4.IsUnspecified() && connectionSettings.IPv4.Unmap().Is4() ||
		connectionSettings.IPv4Subnet.IsValid() && connectionSettings.IPv4Subnet.Addr().Unmap().Is4()
	has6 := connectionSettings.IPv6.IsValid() && !connectionSettings.IPv6.IsUnspecified() && !connectionSettings.IPv6.Unmap().Is4() ||
		connectionSettings.IPv6Subnet.IsValid() && !connectionSettings.IPv6Subnet.Addr().Unmap().Is4()

	if has4 && has6 {
		return newDualStackManager(
			connectionSettings,
			ipcfg.NewV4(),
			ipcfg.NewV6(),
		), nil
	}
	if has4 {
		return newV4Manager(
			connectionSettings,
			ipcfg.NewV4(),
		), nil
	}
	if has6 {
		return newV6Manager(
			connectionSettings,
			ipcfg.NewV6(),
		), nil
	}
	return nil, fmt.Errorf("no valid IPv4 or IPv6 configured")
}
