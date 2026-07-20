package configuration

import "context"

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

type ClientRuntimeControl interface {
	ClientRuntimeConfiguration() (ClientRuntimeConfiguration, error)
}

type ClientControl interface {
	ClientConfigurationControl
	ClientRuntimeControl
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
	Update(peers []ServerPeer)
}

type ServerRuntimeControl interface {
	ServerRuntimeConfiguration() (ServerRuntimeConfiguration, error)
	WatchServerRuntimeConfiguration(
		ctx context.Context,
		revoker ServerSessionRevoker,
		updater ServerAllowedPeersUpdater,
	)
}

type ServerControl interface {
	ServerConfigurationControl
	ServerRuntimeControl
}

type ServerPeer struct {
	Name      string
	PublicKey []byte
	Enabled   bool
	ClientID  int
}
