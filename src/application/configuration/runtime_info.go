package configuration

import (
	"net/netip"
	"tungo/infrastructure/settings"
)

type RuntimeInfo struct {
	Protocol  settings.Protocol
	Endpoints []EndpointInfo
}

type EndpointInfo struct {
	Protocol   settings.Protocol
	Server     settings.Host
	Port       int
	TunnelIPv4 netip.Addr
	TunnelIPv6 netip.Addr
}

func endpointInfoFromSettings(protocol settings.Protocol, s settings.Settings) (EndpointInfo, bool) {
	if s.Server.IsZero() && !s.IPv4.IsValid() && !s.IPv6.IsValid() {
		return EndpointInfo{}, false
	}
	if s.Protocol != settings.UNKNOWN {
		protocol = s.Protocol
	}
	return EndpointInfo{
		Protocol:   protocol,
		Server:     s.Server,
		Port:       s.Port,
		TunnelIPv4: s.IPv4,
		TunnelIPv6: s.IPv6,
	}, true
}
