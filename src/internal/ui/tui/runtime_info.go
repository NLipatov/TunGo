package tui

import (
	clientconfig "tungo/internal/config/client"
	serverconfig "tungo/internal/config/server"
	"tungo/internal/config/settings"
	bubbleTea "tungo/internal/ui/tui/internal/bubble_tea"
)

func clientRuntimeInfo(configuration *clientconfig.Configuration) (bubbleTea.RuntimeInfo, error) {
	active, err := configuration.ActiveSettings()
	if err != nil {
		return bubbleTea.RuntimeInfo{}, err
	}
	info := bubbleTea.RuntimeInfo{Protocol: configuration.Protocol}
	if endpoint, ok := endpointInfo(configuration.Protocol, active); ok {
		info.Endpoints = []bubbleTea.EndpointInfo{endpoint}
	}
	return info, nil
}

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
