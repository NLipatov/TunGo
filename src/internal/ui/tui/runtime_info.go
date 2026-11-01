package tui

import (
	serverconfig "tungo/internal/config/server"
	"tungo/internal/config/settings"
	bubbleTea "tungo/internal/ui/tui/internal/bubble_tea"
)

// clientRuntimeInfo builds runtime information from the client's current settings.
func clientRuntimeInfo(active settings.Settings) bubbleTea.RuntimeInfo {
	info := bubbleTea.RuntimeInfo{Protocol: active.Protocol}
	if endpoint, ok := endpointInfo(active.Protocol, active); ok {
		info.Endpoints = []bubbleTea.EndpointInfo{endpoint}
	}
	return info
}

// serverRuntimeInfo builds runtime information from all enabled server profiles with valid endpoint settings.
func serverRuntimeInfo(configuration *serverconfig.Configuration) bubbleTea.RuntimeInfo {
	endpoints := make([]bubbleTea.EndpointInfo, 0, 3)
	for _, profile := range configuration.Profiles() {
		if !profile.Enabled {
			continue
		}
		if endpoint, ok := endpointInfo(profile.Settings.Protocol, profile.Settings); ok {
			endpoints = append(endpoints, endpoint)
		}
	}
	return bubbleTea.RuntimeInfo{Endpoints: endpoints}
}

// endpointInfo builds endpoint details from protocol settings when they describe a valid endpoint.
// It uses the settings protocol when one is specified, and reports whether endpoint information was found.
func endpointInfo(protocol settings.Protocol, protocolSettings settings.Settings) (bubbleTea.EndpointInfo, bool) {
	if protocolSettings.Server == (settings.Host{}) && !protocolSettings.IPv4.IsValid() && !protocolSettings.IPv6.IsValid() {
		return bubbleTea.EndpointInfo{}, false
	}
	if protocolSettings.Protocol != settings.UNKNOWN {
		protocol = protocolSettings.Protocol
	}
	return bubbleTea.EndpointInfo{
		Protocol:   protocol,
		Server:     protocolSettings.Server,
		Port:       protocolSettings.Port,
		TunnelIPv4: protocolSettings.IPv4,
		TunnelIPv6: protocolSettings.IPv6,
	}, true
}
