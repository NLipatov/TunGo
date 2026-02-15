//go:build darwin

package manager

import (
	"fmt"
	"tungo/infrastructure/PAL/exec_commander"

	"tungo/application/network/routing/tun"
	ifcfg "tungo/infrastructure/PAL/darwin/network_tools/ifconfig"
	rtpkg "tungo/infrastructure/PAL/darwin/network_tools/route"
	"tungo/infrastructure/settings"
)

// Factory builds a TUN manager for darwin: dual-stack, IPv4-only, or IPv6-only.
type Factory struct {
	s          settings.Settings
	ifcFactory *ifcfg.Factory
	rtFactory  *rtpkg.Factory
}

func NewFactory(s settings.Settings) *Factory {
	cmd := exec_commander.NewExecCommander()
	return &Factory{
		s:          s,
		ifcFactory: ifcfg.NewFactory(cmd),
		rtFactory:  rtpkg.NewFactory(cmd),
	}
}

// Create returns a tun.ClientManager for the configured address families.
// Dual-stack is used when both a valid IPv4IP and a valid IPv6IP are configured.
func (f *Factory) Create() (tun.ClientManager, error) {
	has4 := f.s.IPv4IP.IsValid() && !f.s.IPv4IP.IsUnspecified() && f.s.IPv4IP.Unmap().Is4()
	has6 := f.s.IPv6IP.IsValid() && !f.s.IPv6IP.IsUnspecified() && !f.s.IPv6IP.Unmap().Is4()

	if has4 && has6 {
		return newDualStack(
			f.s,
			f.ifcFactory.NewV4(),
			f.ifcFactory.NewV6(),
			f.rtFactory.NewV4(),
			f.rtFactory.NewV6(),
		), nil
	}
	if has4 {
		return newV4(
			f.s,
			f.ifcFactory.NewV4(),
			f.rtFactory.NewV4(),
		), nil
	}
	if has6 {
		return newV6(
			f.s,
			f.ifcFactory.NewV6(),
			f.rtFactory.NewV6(),
		), nil
	}
	return nil, fmt.Errorf("no valid IPv4IP or IPv6IP configured")
}
