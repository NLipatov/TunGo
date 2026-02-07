//go:build darwin

package manager

import (
	"fmt"
	"net"
	"strings"
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
}

func NewFactory(s settings.Settings) *Factory {
	cmd := exec_commander.NewExecCommander()
	return &Factory{
		s:          s,
		ifcFactory: ifcfg.NewFactory(cmd),
		rtFactory:  rtpkg.NewFactory(cmd),
	}
}

// Create returns a tun.ClientManager specialized for IPv4 or IPv6 (darwin).
func (f *Factory) Create() (tun.ClientManager, error) {
	ifAddr := stripZone(f.s.InterfaceIP)
	ip := net.ParseIP(ifAddr)
	if ip == nil {
		return nil, fmt.Errorf("invalid InterfaceIP: %q", f.s.InterfaceIP)
	}
	if ip.IsUnspecified() {
		return nil, fmt.Errorf("unspecified InterfaceIP is not allowed: %q", f.s.InterfaceIP)
	}

	hostIP, ok := f.s.Host.IP()
	if !ok {
		return nil, fmt.Errorf("invalid Host: %q", f.s.Host)
	}
	connIP := net.ParseIP(hostIP.String())
	// Enforce family match to avoid surprising routing behavior.
	if (ip.To4() != nil) != (connIP.To4() != nil) {
		return nil, fmt.Errorf("IP family mismatch: InterfaceIP=%q vs Host=%q",
			f.s.InterfaceIP, f.s.Host)
	}

	if ip.To4() != nil {
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

func stripZone(s string) string {
	if i := strings.IndexByte(s, '%'); i >= 0 {
		return s[:i]
	}
	return s
}
