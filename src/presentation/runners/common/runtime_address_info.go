package common

import (
	"net/netip"
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/settings"
)

type RuntimeAddressPair struct {
	IPv4 netip.Addr
	IPv6 netip.Addr
}

func (p RuntimeAddressPair) IsValid() bool {
	return p.IPv4.IsValid() || p.IPv6.IsValid()
}

func (p RuntimeAddressPair) MergeMissing(from RuntimeAddressPair) RuntimeAddressPair {
	if !p.IPv4.IsValid() && from.IPv4.IsValid() {
		p.IPv4 = from.IPv4
	}
	if !p.IPv6.IsValid() && from.IPv6.IsValid() {
		p.IPv6 = from.IPv6
	}
	return p
}

type RuntimeTunnelAddress struct {
	Protocol settings.Protocol
	Address  RuntimeAddressPair
}

type RuntimeAddressInfo struct {
	ServerAddress   RuntimeAddressPair
	TunnelAddresses []RuntimeTunnelAddress
}

func RuntimeAddressInfoFromClientConfiguration(conf clientConfiguration.Configuration) RuntimeAddressInfo {
	info := RuntimeAddressInfo{}
	activeSettings, err := conf.ActiveSettings()
	if err != nil {
		return info
	}
	info.ServerAddress = runtimeAddressPairFromHost(activeSettings.Server)
	if tunnelAddress, ok := newRuntimeTunnelAddress(
		protocolOrFallback(activeSettings.Protocol, conf.Protocol),
		runtimeAddressPairFromAddrs(activeSettings.IPv4, activeSettings.IPv6),
	); ok {
		info.TunnelAddresses = append(info.TunnelAddresses, tunnelAddress)
	}
	return info
}

func RuntimeAddressInfoFromServerConfiguration(conf serverConfiguration.Configuration) RuntimeAddressInfo {
	info := RuntimeAddressInfo{}
	for _, enabledSetting := range enabledProtocolSettings(conf) {
		s := enabledSetting.settings
		info.ServerAddress = info.ServerAddress.MergeMissing(runtimeAddressPairFromHost(s.Server))
		if tunnelAddress, ok := newRuntimeTunnelAddress(
			protocolOrFallback(s.Protocol, enabledSetting.protocol),
			runtimeAddressPairFromAddrs(s.IPv4, s.IPv6),
		); ok {
			info.TunnelAddresses = append(info.TunnelAddresses, tunnelAddress)
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

func runtimeAddressPairFromHost(host settings.Host) RuntimeAddressPair {
	var pair RuntimeAddressPair
	if ipv4, ok := host.IPv4(); ok {
		pair.IPv4 = ipv4
	}
	if ipv6, ok := host.IPv6(); ok {
		pair.IPv6 = ipv6
	}
	return pair
}

func runtimeAddressPairFromAddrs(ipv4, ipv6 netip.Addr) RuntimeAddressPair {
	return RuntimeAddressPair{
		IPv4: ipv4,
		IPv6: ipv6,
	}
}

func newRuntimeTunnelAddress(protocol settings.Protocol, address RuntimeAddressPair) (RuntimeTunnelAddress, bool) {
	if !address.IsValid() {
		return RuntimeTunnelAddress{}, false
	}
	return RuntimeTunnelAddress{
		Protocol: protocol,
		Address:  address,
	}, true
}

func protocolOrFallback(protocol, fallback settings.Protocol) settings.Protocol {
	if protocol == settings.UNKNOWN {
		return fallback
	}
	return protocol
}
