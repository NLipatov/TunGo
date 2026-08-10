package configuration

import (
	"context"

	clientConfiguration "tungo/application/configuration/client"
	serverConfiguration "tungo/application/configuration/server"
)

type Controls struct {
	Client ClientControl
	Server ServerControl
}

func (c Controls) ServerSupported() bool {
	return c.Server != nil
}

type ClientConfigurationControl interface {
	List() ([]string, error)
	Select(path string) error
	ValidateActive() error
	RuntimeInfo() (RuntimeInfo, error)
	CreateFromJSON(name, rawJSON string) error
	Delete(path string) error
}

type ClientControl interface {
	ClientConfigurationControl
	Configuration() (*clientConfiguration.Configuration, error)
}

type ServerConfigurationControl interface {
	RuntimeInfo() (RuntimeInfo, error)
	GenerateClientConfiguration() (GeneratedClientConfiguration, error)
	ListPeers() ([]ServerPeer, error)
	SetPeerEnabled(clientID int, enabled bool) error
	RemovePeer(clientID int) error
}

type GeneratedClientConfiguration struct {
	JSON string
	Path string
}

type ServerSessionRevoker interface {
	RevokeByPubKey(pubKey []byte) int
}

type ServerAllowedPeersUpdater interface {
	Update(peers []serverConfiguration.AllowedPeer)
}

type ServerRuntimeControl interface {
	ServerConfiguration() (*serverConfiguration.Configuration, error)
	WatchServerConfiguration(
		ctx context.Context,
		revoker ServerSessionRevoker,
		updater ServerAllowedPeersUpdater,
	)
}

type ServerControl interface {
	ServerConfigurationControl
	ServerRuntimeControl
}

// ServerPeer remains as a source-compatible name for the persisted peer type.
type ServerPeer = serverConfiguration.AllowedPeer
