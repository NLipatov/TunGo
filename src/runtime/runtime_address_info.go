package runtime

import (
	"net/netip"
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/settings"
)

type EndpointInfo struct {
	Protocol   settings.Protocol
	Server     settings.Host
	Port       int
	TunnelIPv4 netip.Addr
	TunnelIPv6 netip.Addr
}

func EndpointInfoFromClientConfiguration(conf clientConfiguration.Configuration) []EndpointInfo {
	activeSettings, err := conf.ActiveSettings()
	if err != nil {
		return nil
	}
	if endpoint, ok := newEndpointInfo(
		protocolOrFallback(activeSettings.Protocol, conf.Protocol),
		activeSettings.Server,
		activeSettings.Port,
		activeSettings.IPv4,
		activeSettings.IPv6,
	); ok {
		return []EndpointInfo{endpoint}
	}
	return nil
}

func EndpointInfoFromServerConfiguration(conf serverConfiguration.Configuration) []EndpointInfo {
	endpoints := make([]EndpointInfo, 0, 3)
	for _, enabledSetting := range enabledProtocolSettings(conf) {
		s := enabledSetting.settings
		if endpoint, ok := newEndpointInfo(
			protocolOrFallback(s.Protocol, enabledSetting.protocol),
			s.Server,
			s.Port,
			s.IPv4,
			s.IPv6,
		); ok {
			endpoints = append(endpoints, endpoint)
		}
	}
	return endpoints
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

func newEndpointInfo(
	protocol settings.Protocol,
	server settings.Host,
	port int,
	tunnelIPv4 netip.Addr,
	tunnelIPv6 netip.Addr,
) (EndpointInfo, bool) {
	if server.IsZero() && !tunnelIPv4.IsValid() && !tunnelIPv6.IsValid() {
		return EndpointInfo{}, false
	}
	return EndpointInfo{
		Protocol:   protocol,
		Server:     server,
		Port:       port,
		TunnelIPv4: tunnelIPv4,
		TunnelIPv6: tunnelIPv6,
	}, true
}

func protocolOrFallback(protocol, fallback settings.Protocol) settings.Protocol {
	if protocol == settings.UNKNOWN {
		return fallback
	}
	return protocol
}
