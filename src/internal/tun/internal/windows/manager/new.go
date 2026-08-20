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
	AddHostRouteViaGateway(hostIP netip.Addr, ifName string, gateway netip.Addr) error
	AddHostRouteOnLink(hostIP netip.Addr, ifName string) error
	AddDefaultSplitRoutes(ifName string) error
	DeleteDefaultSplitRoutes(ifName string) error
	DeleteRoute(destination netip.Addr) error
	DeleteRouteOnInterface(destination netip.Addr, ifName string) error
	BestRoute(dest netip.Addr) (netip.Addr, string, int, int, error)
}

func New(s settings.Settings) (clientManager, error) {
	has4 := s.IPv4Subnet.IsValid() &&
		s.IPv4Subnet.Addr().Unmap().Is4()
	has6 := s.IPv6Subnet.IsValid() &&
		s.IPv6Subnet.Addr().Unmap().Is6()

	if has4 && has6 {
		return newDualStackManager(
			s,
			ipcfg.NewV4(),
			ipcfg.NewV6(),
		), nil
	}
	if has4 {
		return newV4Manager(
			s,
			ipcfg.NewV4(),
		), nil
	}
	if has6 {
		return newV6Manager(
			s,
			ipcfg.NewV6(),
		), nil
	}
	return nil, fmt.Errorf("no valid IPv4 or IPv6 configured")
}
