package configuration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	appConfgen "tungo/application/confgen"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/host_resolver"
)

type serverControl struct {
	manager serverConfiguration.ConfigurationManager
}

func (c *serverControl) GenerateClientConfiguration() (string, error) {
	gen := appConfgen.NewGenerator(c.manager, &primitives.DefaultKeyDeriver{}, host_resolver.NewDialResolver())
	conf, err := gen.Generate()
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal client configuration: %w", err)
	}
	path, err := writeServerClientConfigFile(conf.ClientID, data)
	if err != nil {
		return "", fmt.Errorf("failed to save client configuration: %w", err)
	}
	return path, nil
}

func (c *serverControl) ListPeers() ([]ServerPeer, error) {
	peers, err := c.manager.ListAllowedPeers()
	if err != nil {
		return nil, err
	}
	result := make([]ServerPeer, len(peers))
	for i := range peers {
		result[i] = ServerPeer{
			Name:      peers[i].Name,
			PublicKey: append([]byte(nil), peers[i].PublicKey...),
			Enabled:   peers[i].Enabled,
			ClientID:  peers[i].ClientID,
		}
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
	configPath, err := serverConfiguration.NewServerResolver().Resolve()
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
