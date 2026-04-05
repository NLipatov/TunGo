package common

import (
	"net/netip"
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/settings"
)

type RuntimeTunnelAddress struct {
	Protocol settings.Protocol
	IPv4     netip.Addr
	IPv6     netip.Addr
}

type RuntimeAddressInfo struct {
	ServerIPv4      netip.Addr
	ServerIPv6      netip.Addr
	TunnelIPv4      netip.Addr
	TunnelIPv6      netip.Addr
	TunnelAddresses []RuntimeTunnelAddress
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
	if tunnelAddress, ok := newRuntimeTunnelAddress(protocolOrFallback(activeSettings.Protocol, conf.Protocol), activeSettings.IPv4, activeSettings.IPv6); ok {
		info.TunnelAddresses = append(info.TunnelAddresses, tunnelAddress)
	}
	return info
}

func RuntimeAddressInfoFromServerConfiguration(conf serverConfiguration.Configuration) RuntimeAddressInfo {
	info := RuntimeAddressInfo{}
	for _, enabledSetting := range enabledProtocolSettings(conf) {
		s := enabledSetting.settings
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
		if tunnelAddress, ok := newRuntimeTunnelAddress(protocolOrFallback(s.Protocol, enabledSetting.protocol), s.IPv4, s.IPv6); ok {
			info.TunnelAddresses = append(info.TunnelAddresses, tunnelAddress)
			if !info.TunnelIPv4.IsValid() && tunnelAddress.IPv4.IsValid() {
				info.TunnelIPv4 = tunnelAddress.IPv4
			}
			if !info.TunnelIPv6.IsValid() && tunnelAddress.IPv6.IsValid() {
				info.TunnelIPv6 = tunnelAddress.IPv6
			}
		}
	}
	return info
}

type protocolSettings struct {
	protocol settings.Protocol
	settings settings.Settings
}

func enabledProtocolSettings(conf serverConfiguration.Configuration) []protocolSettings {
	result := make([]protocolSettings, 0, 3)
	if conf.EnableTCP {
		result = append(result, protocolSettings{protocol: settings.TCP, settings: conf.TCPSettings})
	}
	if conf.EnableUDP {
		result = append(result, protocolSettings{protocol: settings.UDP, settings: conf.UDPSettings})
	}
	if conf.EnableWS {
		result = append(result, protocolSettings{protocol: settings.WS, settings: conf.WSSettings})
	}
	return result
}

func newRuntimeTunnelAddress(protocol settings.Protocol, ipv4, ipv6 netip.Addr) (RuntimeTunnelAddress, bool) {
	if !ipv4.IsValid() && !ipv6.IsValid() {
		return RuntimeTunnelAddress{}, false
	}
	return RuntimeTunnelAddress{
		Protocol: protocol,
		IPv4:     ipv4,
		IPv6:     ipv6,
	}, true
}

func protocolOrFallback(protocol, fallback settings.Protocol) settings.Protocol {
	if protocol == settings.UNKNOWN {
		return fallback
	}
	return protocol
}
