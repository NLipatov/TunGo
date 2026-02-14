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
// Dual-stack is used when InterfaceIP is IPv4 and a valid IPv6IP is also configured.
func (f *Factory) Create() (tun.ClientManager, error) {
	ifAddr := f.s.InterfaceIP
	if !ifAddr.IsValid() {
		return nil, fmt.Errorf("invalid InterfaceIP: %q", ifAddr)
	}
	if ifAddr.IsUnspecified() {
		return nil, fmt.Errorf("unspecified InterfaceIP is not allowed: %q", ifAddr)
	}

	ifIs4 := ifAddr.Unmap().Is4()

	// Dual-stack: IPv4 interface + valid IPv6 configured.
	if ifIs4 && f.s.IPv6IP.IsValid() && !f.s.IPv6IP.IsUnspecified() {
		return newDualStack(
			f.s,
			f.ifcFactory.NewV4(),
			f.ifcFactory.NewV6(),
			f.rtFactory.NewV4(),
			f.rtFactory.NewV6(),
		), nil
	}

	if ifIs4 {
		return newV4(
			f.s,
			f.ifcFactory.NewV4(),
			f.rtFactory.NewV4(),
		), nil
	}
	return newV6(
		f.s,
		f.ifcFactory.NewV6(),
		f.rtFactory.NewV6(),
	), nil
}
