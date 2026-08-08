package configuration

import "tungo/application/configuration/settings"

type ClientRuntimeConfiguration struct {
	Settings         settings.Settings
	CleanupSettings  []settings.Settings
	X25519PublicKey  []byte
	ClientPublicKey  []byte
	ClientPrivateKey []byte
}

type ServerRuntimeConfiguration struct {
	TCPSettings      settings.Settings
	UDPSettings      settings.Settings
	WSSettings       settings.Settings
	X25519PublicKey  []byte
	X25519PrivateKey []byte
	ClientCounter    int
	EnableTCP        bool
	EnableUDP        bool
	EnableWS         bool
	AllowedPeers     []ServerPeer
}

func (c ServerRuntimeConfiguration) Profiles() [3]settings.Profile {
	return [3]settings.Profile{
		{Settings: c.TCPSettings, Enabled: c.EnableTCP},
		{Settings: c.UDPSettings, Enabled: c.EnableUDP},
		{Settings: c.WSSettings, Enabled: c.EnableWS},
	}
}
