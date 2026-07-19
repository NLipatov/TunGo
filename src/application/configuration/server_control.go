package configuration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	appConfgen "tungo/application/configuration/internal/confgen"
	serverImplementation "tungo/application/configuration/internal/server"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/logging"
	"tungo/infrastructure/network/host_resolver"
	"tungo/infrastructure/settings"

	"log/slog"
)

type serverControl struct {
	resolver pathResolver
	manager  serverConfigurationManager
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

func (c *serverControl) ServerRuntimeConfiguration() (ServerRuntimeConfiguration, error) {
	if err := serverImplementation.NewX25519KeyManager(c.manager).PrepareKeys(); err != nil {
		return ServerRuntimeConfiguration{}, fmt.Errorf("could not prepare server keys: %w", err)
	}
	conf, err := c.manager.Configuration()
	if err != nil {
		return ServerRuntimeConfiguration{}, err
	}
	peers := make([]ServerPeer, len(conf.AllowedPeers))
	for i := range conf.AllowedPeers {
		peers[i] = serverPeer(conf.AllowedPeers[i])
	}
	return ServerRuntimeConfiguration{
		TCPSettings:           conf.TCPSettings,
		UDPSettings:           conf.UDPSettings,
		WSSettings:            conf.WSSettings,
		FallbackServerAddress: conf.FallbackServerAddress,
		X25519PublicKey:       append([]byte(nil), conf.X25519PublicKey...),
		X25519PrivateKey:      append([]byte(nil), conf.X25519PrivateKey...),
		ClientCounter:         conf.ClientCounter,
		EnableTCP:             conf.EnableTCP,
		EnableUDP:             conf.EnableUDP,
		EnableWS:              conf.EnableWS,
		AllowedPeers:          peers,
	}, nil
}

func (c *serverControl) WatchServerRuntimeConfiguration(
	ctx context.Context,
	revoker ServerSessionRevoker,
	updater ServerAllowedPeersUpdater,
) {
	configPath, _ := c.resolver.Resolve()
	watcher := serverImplementation.NewConfigWatcher(
		c.manager,
		revoker,
		allowedPeersUpdater{updater: updater},
		configPath,
		serverImplementation.DefaultWatchInterval,
		logging.NewStdLogger(slog.LevelInfo),
	)
	watcher.Watch(ctx)
}

type allowedPeersUpdater struct {
	updater ServerAllowedPeersUpdater
}

func (a allowedPeersUpdater) Update(peers []serverConfiguration.AllowedPeer) {
	if a.updater == nil {
		return
	}
	result := make([]ServerPeer, len(peers))
	for i := range peers {
		result[i] = serverPeer(peers[i])
	}
	a.updater.Update(result)
}

func serverPeer(peer serverConfiguration.AllowedPeer) ServerPeer {
	return ServerPeer{
		Name:      peer.Name,
		PublicKey: append([]byte(nil), peer.PublicKey...),
		Enabled:   peer.Enabled,
		ClientID:  peer.ClientID,
	}
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
	path, err := writeServerClientConfigFile(conf.ClientID, data)
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
	result := make([]ServerPeer, len(peers))
	for i := range peers {
		result[i] = serverPeer(peers[i])
	}
	return result, nil
}

func (c *serverControl) SetPeerEnabled(clientID int, enabled bool) error {
	return c.manager.SetAllowedPeerEnabled(clientID, enabled)
}

func (c *serverControl) RemovePeer(clientID int) error {
	return c.manager.RemoveAllowedPeer(clientID)
}

func writeServerClientConfigFile(clientID int, data []byte) (string, error) {
	configPath, err := serverImplementation.NewServerResolver().Resolve()
	if err != nil {
		return "", fmt.Errorf("failed to resolve server config path: %w", err)
	}
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to create server config directory: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("client_configuration.json.%d", clientID))
	return path, os.WriteFile(path, data, 0600)
}
