package bubble_tea

import (
	"net/netip"

	clientconfig "tungo/internal/config/client"
	serverconfig "tungo/internal/config/server"
	"tungo/internal/config/settings"
)

type ClientConfigurations interface {
	Active() (*clientconfig.Configuration, error)
	List() ([]string, error)
	Import(name, rawJSON string) error
	Activate(name string) error
	Delete(name string) error
}

type ServerConfigurations interface {
	GenerateClient() (serverconfig.GeneratedClient, error)
	Peers() ([]serverconfig.AllowedPeer, error)
	SetPeerEnabled(clientID int, enabled bool) error
	RemovePeer(clientID int) error
}

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
