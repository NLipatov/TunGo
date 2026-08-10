package configuration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	appConfgen "tungo/application/configuration/internal/confgen"
	serverImplementation "tungo/application/configuration/internal/server"
	serverConfiguration "tungo/application/configuration/server"
	"tungo/application/configuration/settings"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/host_resolver"
)

type serverControl struct {
	configPath string
	manager    serverConfigurationManager
}

type serverConfigurationManager interface {
	Configuration() (*serverConfiguration.Configuration, error)
	IncrementClientCounter() error
	InjectX25519Keys(public, private []byte) error
	AddAllowedPeer(peer serverConfiguration.AllowedPeer) error
	ListAllowedPeers() ([]serverConfiguration.AllowedPeer, error)
	SetAllowedPeerEnabled(clientID int, enabled bool) error
	RemoveAllowedPeer(clientID int) error
	EnsureIPv6Subnets() error
	InvalidateCache()
}

func (c *serverControl) ServerConfiguration() (*serverConfiguration.Configuration, error) {
	if err := serverImplementation.NewX25519KeyManager(c.manager).PrepareKeys(); err != nil {
		return nil, fmt.Errorf("could not prepare server keys: %w", err)
	}
	conf, err := c.manager.Configuration()
	if err != nil {
		return nil, err
	}
	return conf, nil
}

func (c *serverControl) WatchServerConfiguration(
	ctx context.Context,
	revoker ServerSessionRevoker,
	updater ServerAllowedPeersUpdater,
) {
	watcher := serverImplementation.NewConfigWatcher(
		c.manager,
		revoker,
		updater,
		c.configPath,
		serverImplementation.DefaultWatchInterval,
	)
	watcher.Watch(ctx)
}

func (c *serverControl) RuntimeInfo() (RuntimeInfo, error) {
	conf, err := c.manager.Configuration()
	if err != nil {
		return RuntimeInfo{}, err
	}

	endpoints := make([]EndpointInfo, 0, 3)
	if conf.EnableTCP {
		if endpoint, ok := endpointInfoFromSettings(settings.TCP, conf.TCPSettings); ok {
			endpoints = append(endpoints, endpoint)
		}
	}
	if conf.EnableUDP {
		if endpoint, ok := endpointInfoFromSettings(settings.UDP, conf.UDPSettings); ok {
			endpoints = append(endpoints, endpoint)
		}
	}
	if conf.EnableWS {
		if endpoint, ok := endpointInfoFromSettings(settings.WS, conf.WSSettings); ok {
			endpoints = append(endpoints, endpoint)
		}
	}
	return RuntimeInfo{Endpoints: endpoints}, nil
}

func (c *serverControl) GenerateClientConfiguration() (GeneratedClientConfiguration, error) {
	if err := serverImplementation.NewX25519KeyManager(c.manager).PrepareKeys(); err != nil {
		return GeneratedClientConfiguration{}, fmt.Errorf("could not prepare server keys: %w", err)
	}
	gen := appConfgen.NewGenerator(c.manager, &primitives.DefaultKeyDeriver{}, host_resolver.NewDialResolver())
	conf, err := gen.Generate()
	if err != nil {
		return GeneratedClientConfiguration{}, err
	}
	data, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return GeneratedClientConfiguration{}, fmt.Errorf("failed to marshal client configuration: %w", err)
	}
	path, err := writeServerClientConfigFile(c.configPath, conf.ClientID, data)
	if err != nil {
		return GeneratedClientConfiguration{}, fmt.Errorf("failed to save client configuration: %w", err)
	}
	return GeneratedClientConfiguration{JSON: string(data), Path: path}, nil
}

func (c *serverControl) ListPeers() ([]ServerPeer, error) {
	peers, err := c.manager.ListAllowedPeers()
	if err != nil {
		return nil, err
	}
	for i := range peers {
		peers[i].PublicKey = append([]byte(nil), peers[i].PublicKey...)
	}
	return peers, nil
}

func (c *serverControl) SetPeerEnabled(clientID int, enabled bool) error {
	return c.manager.SetAllowedPeerEnabled(clientID, enabled)
}

func (c *serverControl) RemovePeer(clientID int) error {
	return c.manager.RemoveAllowedPeer(clientID)
}

func writeServerClientConfigFile(configPath string, clientID int, data []byte) (string, error) {
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to create server config directory: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("client_configuration.json.%d", clientID))
	return path, os.WriteFile(path, data, 0600)
}
