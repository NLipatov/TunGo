//go:build windows

package manager

import (
	"fmt"
	"net"
	"strings"
	"tungo/infrastructure/PAL/windows/network_tools/ipconfig"
	"tungo/infrastructure/PAL/windows/network_tools/netsh"
	"tungo/infrastructure/PAL/windows/network_tools/route"
	"tungo/infrastructure/settings"

	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL"
)

// Factory builds a family-specific TUN manager (IPv4 or IPv6) based on InterfaceAddress.
// Assumptions:
//   - InterfaceAddress is a strict invariant and always valid (non-empty, not unspecified).
//   - We keep code simple: choose by InterfaceAddress family only.
//   - Optional safety: ensure ConnectionIP family matches InterfaceAddress family.
type Factory struct {
	connectionSettings settings.Settings
	commander          PAL.Commander
	netshFactory       netsh.Factory
	routeFactory       route.Factory
}

func NewFactory(
	connectionSettings settings.Settings,
	commander PAL.Commander,
	netshFactory netsh.Factory,
	routeFactory route.Factory,
) *Factory {
	return &Factory{
		connectionSettings: connectionSettings,
		commander:          commander,
		netshFactory:       netshFactory,
		routeFactory:       routeFactory,
	}
}

// Create returns a tun.ClientManager specialized for IPv4 or IPv6.
func (f *Factory) Create() (tun.ClientManager, error) {
	ifAddr := f.stripZone(f.connectionSettings.InterfaceAddress) // e.g., "fe80::1%12" -> "fe80::1"
	ip := net.ParseIP(ifAddr)
	if ip == nil {
		return nil, fmt.Errorf("invalid InterfaceAddress: %q", f.connectionSettings.InterfaceAddress)
	}
	if ip.IsUnspecified() {
		return nil, fmt.Errorf("unspecified InterfaceAddress is not allowed: %q", f.connectionSettings.InterfaceAddress)
	}
	// Optional safety: enforce family match between InterfaceAddress and ConnectionIP.
	connIP := net.ParseIP(f.stripZone(f.connectionSettings.ConnectionIP))
	if connIP == nil {
		return nil, fmt.Errorf("invalid ConnectionIP: %q", f.connectionSettings.ConnectionIP)
	}
	if (ip.To4() != nil) != (connIP.To4() != nil) {
		return nil, fmt.Errorf("IP family mismatch: InterfaceAddress=%q vs ConnectionIP=%q",
			f.connectionSettings.InterfaceAddress, f.connectionSettings.ConnectionIP)
	}
	if ip.To4() != nil {
		return newV4Manager(
			f.connectionSettings,
			f.netshFactory.CreateNetshV4(),
			f.routeFactory.CreateRouteV4(),
			ipconfig.NewWrapper(f.commander),
		), nil
	} else {
		return newV6Manager(
			f.connectionSettings,
			f.netshFactory.CreateNetshV6(),
			f.routeFactory.CreateRouteV6(),
			ipconfig.NewWrapper(f.commander),
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
