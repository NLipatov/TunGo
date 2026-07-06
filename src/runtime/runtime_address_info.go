package runtime

import (
	"net/netip"
	"tungo/infrastructure/settings"
)

type EndpointInfo struct {
	Protocol   settings.Protocol
	Server     settings.Host
	Port       int
	TunnelIPv4 netip.Addr
	TunnelIPv6 netip.Addr
}

func EndpointInfoFromSettings(protocol settings.Protocol, s settings.Settings) (EndpointInfo, bool) {
	if s.Server.IsZero() && !s.IPv4.IsValid() && !s.IPv6.IsValid() {
		return EndpointInfo{}, false
	}
	return EndpointInfo{
		Protocol:   protocolOrFallback(s.Protocol, protocol),
		Server:     s.Server,
		Port:       s.Port,
		TunnelIPv4: s.IPv4,
		TunnelIPv6: s.IPv6,
	}, true
}

func protocolOrFallback(protocol, fallback settings.Protocol) settings.Protocol {
	if protocol == settings.UNKNOWN {
		return fallback
	}
	return protocol
}
