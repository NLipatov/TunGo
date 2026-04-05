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

type RuntimeProtocolAddress struct {
	Protocol      settings.Protocol
	ServerAddress RuntimeAddressPair
	TunnelAddress RuntimeAddressPair
}

type RuntimeAddressInfo struct {
	ProtocolAddresses []RuntimeProtocolAddress
}

func RuntimeAddressInfoFromClientConfiguration(conf clientConfiguration.Configuration) RuntimeAddressInfo {
	info := RuntimeAddressInfo{}
	activeSettings, err := conf.ActiveSettings()
	if err != nil {
		return info
	}
	if protocolAddress, ok := newRuntimeProtocolAddress(
		protocolOrFallback(activeSettings.Protocol, conf.Protocol),
		runtimeAddressPairFromHost(activeSettings.Server),
		runtimeAddressPairFromAddrs(activeSettings.IPv4, activeSettings.IPv6),
	); ok {
		info.ProtocolAddresses = append(info.ProtocolAddresses, protocolAddress)
	}
	return info
}

func RuntimeAddressInfoFromServerConfiguration(conf serverConfiguration.Configuration) RuntimeAddressInfo {
	info := RuntimeAddressInfo{}
	for _, enabledSetting := range enabledProtocolSettings(conf) {
		s := enabledSetting.settings
		if protocolAddress, ok := newRuntimeProtocolAddress(
			protocolOrFallback(s.Protocol, enabledSetting.protocol),
			runtimeAddressPairFromHost(s.Server),
			runtimeAddressPairFromAddrs(s.IPv4, s.IPv6),
		); ok {
			info.ProtocolAddresses = append(info.ProtocolAddresses, protocolAddress)
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

func newRuntimeProtocolAddress(
	protocol settings.Protocol,
	serverAddress RuntimeAddressPair,
	tunnelAddress RuntimeAddressPair,
) (RuntimeProtocolAddress, bool) {
	if !serverAddress.IsValid() && !tunnelAddress.IsValid() {
		return RuntimeProtocolAddress{}, false
	}
	return RuntimeProtocolAddress{
		Protocol:      protocol,
		ServerAddress: serverAddress,
		TunnelAddress: tunnelAddress,
	}, true
}

func protocolOrFallback(protocol, fallback settings.Protocol) settings.Protocol {
	if protocol == settings.UNKNOWN {
		return fallback
	}
	return protocol
}
