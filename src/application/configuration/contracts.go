package configuration

type Controls struct {
	Client ClientConfigurationControl
	Server ServerConfigurationControl
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

type ServerConfigurationControl interface {
	RuntimeInfo() (RuntimeInfo, error)
	GenerateClientConfiguration() (string, error)
	ListPeers() ([]ServerPeer, error)
	SetPeerEnabled(clientID int, enabled bool) error
	RemovePeer(clientID int) error
}

type ServerPeer struct {
	Name      string
	PublicKey []byte
	Enabled   bool
	ClientID  int
}
