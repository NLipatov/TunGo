//go:build darwin

package manager

import (
	"fmt"
	"io"
	"net/netip"
	"tungo/internal/platform/command"

	"tungo/internal/config/settings"
	ifcfg "tungo/internal/tun/internal/darwin/ifconfig"
	rtpkg "tungo/internal/tun/internal/darwin/route"
)

type clientManager interface {
	OpenTunnel(serverAddr netip.Addr) (io.ReadWriteCloser, error)
	CloseTunnel() error
}

type interfaceConfigurator interface {
	LinkAddrAdd(ifName string, prefix netip.Prefix) error
	SetMTU(ifName string, mtu int) error
}

type routeConfigurator interface {
	Add(destIP string) error
	AddSplit(ifName string) error
	DelSplit(ifName string) error
	Del(destIP string) error
}

func New(s settings.Settings) (clientManager, error) {
	cmd := command.New()
	has4 := s.IPv4Subnet.IsValid() &&
		s.IPv4Subnet.Addr().Unmap().Is4()
	has6 := s.IPv6Subnet.IsValid() &&
		s.IPv6Subnet.Addr().Unmap().Is6()

	if has4 && has6 {
		return newDualStack(
			s,
			ifcfg.NewV4(cmd),
			ifcfg.NewV6(cmd),
			rtpkg.NewV4(cmd),
			rtpkg.NewV6(cmd),
		), nil
	}
	if has4 {
		return newV4(
			s,
			ifcfg.NewV4(cmd),
			rtpkg.NewV4(cmd),
		), nil
	}
	if has6 {
		return newV6(
			s,
			ifcfg.NewV6(cmd),
			rtpkg.NewV6(cmd),
		), nil
	}
	return nil, fmt.Errorf("no valid IPv4 or IPv6 configured")
}
