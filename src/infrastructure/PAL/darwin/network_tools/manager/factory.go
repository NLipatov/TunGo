//go:build darwin

package manager

import (
	"fmt"
	"net"
	"strings"
	"tungo/infrastructure/PAL/darwin/network_tools/scutil"
	"tungo/infrastructure/PAL/exec_commander"

	"tungo/application/network/routing/tun"
	ifcfg "tungo/infrastructure/PAL/darwin/network_tools/ifconfig"
	rtpkg "tungo/infrastructure/PAL/darwin/network_tools/route"
	"tungo/infrastructure/settings"
)

// Factory builds a family-specific TUN manager (IPv4 or IPv6) for darwin.
type Factory struct {
	s          settings.Settings
	ifcFactory *ifcfg.Factory
	rtFactory  *rtpkg.Factory
	scFactory  *scutil.Factory
}

func NewFactory(s settings.Settings) *Factory {
	cmd := exec_commander.NewExecCommander()
	return &Factory{
		s:          s,
		ifcFactory: ifcfg.NewFactory(cmd),
		rtFactory:  rtpkg.NewFactory(cmd),
		scFactory:  scutil.NewFactory(),
	}
}

// Create returns a tun.ClientManager specialized for IPv4 or IPv6 (darwin).
func (f *Factory) Create() (tun.ClientManager, error) {
	ifAddr := stripZone(f.s.InterfaceAddress)
	ip := net.ParseIP(ifAddr)
	if ip == nil {
		return nil, fmt.Errorf("invalid InterfaceAddress: %q", f.s.InterfaceAddress)
	}
	if ip.IsUnspecified() {
		return nil, fmt.Errorf("unspecified InterfaceAddress is not allowed: %q", f.s.InterfaceAddress)
	}

	connIP := net.ParseIP(stripZone(f.s.ConnectionIP))
	if connIP == nil {
		return nil, fmt.Errorf("invalid ConnectionIP: %q", f.s.ConnectionIP)
	}
	// Enforce family match to avoid surprising routing behavior.
	if (ip.To4() != nil) != (connIP.To4() != nil) {
		return nil, fmt.Errorf("IP family mismatch: InterfaceAddress=%q vs ConnectionIP=%q",
			f.s.InterfaceAddress, f.s.ConnectionIP)
	}

	if ip.To4() != nil {
		return newV4(
			f.s,
			f.ifcFactory.NewV4(),
			f.rtFactory.NewV4(),
			f.scFactory.NewV4(),
		), nil
	}
	return newV6(
		f.s,
		f.ifcFactory.NewV6(),
		f.rtFactory.NewV6(),
		f.scFactory.NewV6(),
	), nil
}

func stripZone(s string) string {
	if i := strings.IndexByte(s, '%'); i >= 0 {
		return s[:i]
	}
	return s
}
