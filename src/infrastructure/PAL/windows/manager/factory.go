//go:build windows

package manager

import (
	"fmt"
	"net"
	"strings"
	"tungo/infrastructure/PAL/windows/ipcfg"
	"tungo/infrastructure/settings"

	"tungo/application/network/routing/tun"
)

// Factory builds a family-specific TUN manager (IPv4 or IPv6) based on InterfaceIP.
// Assumptions:
//   - InterfaceIP is a strict invariant and always valid (non-empty, not unspecified).
//   - We keep code simple: choose by InterfaceIP family only.
//   - Optional safety: ensure Host family matches InterfaceIP family.
type Factory struct {
	connectionSettings settings.Settings
	netConfigFactory   ipcfg.Factory
}

func NewFactory(
	connectionSettings settings.Settings,
) *Factory {
	return &Factory{
		connectionSettings: connectionSettings,
		netConfigFactory:   ipcfg.NewFactory(),
	}
}

// Create returns a tun.ClientManager specialized for IPv4 or IPv6.
func (f *Factory) Create() (tun.ClientManager, error) {
	ifAddr := f.stripZone(f.connectionSettings.InterfaceIP) // e.g., "fe80::1%12" -> "fe80::1"
	ip := net.ParseIP(ifAddr)
	if ip == nil {
		return nil, fmt.Errorf("invalid InterfaceIP: %q", f.connectionSettings.InterfaceIP)
	}
	if ip.IsUnspecified() {
		return nil, fmt.Errorf("unspecified InterfaceIP is not allowed: %q", f.connectionSettings.InterfaceIP)
	}
	// Optional safety: enforce family match between InterfaceIP and Host.
	connIP := net.ParseIP(f.stripZone(f.connectionSettings.Host))
	if connIP == nil {
		return nil, fmt.Errorf("invalid Host: %q", f.connectionSettings.Host)
	}
	if (ip.To4() != nil) != (connIP.To4() != nil) {
		return nil, fmt.Errorf("IP family mismatch: InterfaceIP=%q vs Host=%q",
			f.connectionSettings.InterfaceIP, f.connectionSettings.Host)
	}
	if ip.To4() != nil {
		return newV4Manager(
			f.connectionSettings,
			f.netConfigFactory.NewV4(),
		), nil
	} else {
		return newV6Manager(
			f.connectionSettings,
			f.netConfigFactory.NewV6(),
		), nil
	}
}

// stripZone removes IPv6 zone suffix (e.g., "%12") if present.
// Safe to call for IPv4 or empty strings.
func (f *Factory) stripZone(s string) string {
	if i := strings.IndexByte(s, '%'); i >= 0 {
		return s[:i]
	}
	return s
}
