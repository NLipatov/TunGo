package common

import (
	"net/netip"
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
)

type RuntimeAddressInfo struct {
	ServerIPv4 netip.Addr
	ServerIPv6 netip.Addr
	TunnelIPv4 netip.Addr
	TunnelIPv6 netip.Addr
}

func RuntimeAddressInfoFromClientConfiguration(conf clientConfiguration.Configuration) RuntimeAddressInfo {
	info := RuntimeAddressInfo{}
	activeSettings, err := conf.ActiveSettings()
	if err != nil {
		return info
	}
	if serverIPv4, ok := activeSettings.Server.IPv4(); ok {
		info.ServerIPv4 = serverIPv4
	}
	if serverIPv6, ok := activeSettings.Server.IPv6(); ok {
		info.ServerIPv6 = serverIPv6
	}
	if activeSettings.IPv4.IsValid() {
		info.TunnelIPv4 = activeSettings.IPv4
	}
	if activeSettings.IPv6.IsValid() {
		info.TunnelIPv6 = activeSettings.IPv6
	}
	return info
}

func RuntimeAddressInfoFromServerConfiguration(conf serverConfiguration.Configuration) RuntimeAddressInfo {
	info := RuntimeAddressInfo{}
	for _, s := range conf.EnabledSettings() {
		if !info.ServerIPv4.IsValid() {
			if serverIPv4, ok := s.Server.IPv4(); ok {
				info.ServerIPv4 = serverIPv4
			}
		}
		if !info.ServerIPv6.IsValid() {
			if serverIPv6, ok := s.Server.IPv6(); ok {
				info.ServerIPv6 = serverIPv6
			}
		}
		if !info.TunnelIPv4.IsValid() && s.IPv4.IsValid() {
			info.TunnelIPv4 = s.IPv4
		}
		if !info.TunnelIPv6.IsValid() && s.IPv6.IsValid() {
			info.TunnelIPv6 = s.IPv6
		}
		if info.ServerIPv4.IsValid() && info.ServerIPv6.IsValid() && info.TunnelIPv4.IsValid() && info.TunnelIPv6.IsValid() {
			break
		}
	}
	return info
}
