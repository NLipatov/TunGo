//go:build darwin

package manager

import (
	"fmt"
	"tungo/infrastructure/PAL/exec_commander"

	"tungo/application/network/routing/tun"
	ifcfg "tungo/infrastructure/PAL/network/darwin/ifconfig"
	rtpkg "tungo/infrastructure/PAL/network/darwin/route"
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
// Dual-stack is used when both a valid IPv4 and a valid IPv6 are configured.
func (f *Factory) Create() (tun.ClientManager, error) {
	has4 := f.s.IPv4.IsValid() && !f.s.IPv4.IsUnspecified() && f.s.IPv4.Unmap().Is4()
	has6 := f.s.IPv6.IsValid() && !f.s.IPv6.IsUnspecified() && !f.s.IPv6.Unmap().Is4()

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
	return nil, fmt.Errorf("no valid IPv4 or IPv6 configured")
}
