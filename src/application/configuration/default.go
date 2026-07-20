package configuration

import (
	"fmt"
	"path/filepath"

	clientConfiguration "tungo/application/configuration/internal/client"
	serverConfiguration "tungo/application/configuration/internal/server"
	"tungo/infrastructure/PAL/platform"
	"tungo/infrastructure/PAL/stat"
)

func DefaultStorageDirectory() (string, error) {
	path, err := clientConfiguration.NewDefaultResolver().Resolve()
	if err != nil {
		return "", err
	}
	return filepath.Dir(path), nil
}

func NewDefaultClientControl() ClientControl {
	clientResolver := clientConfiguration.NewDefaultResolver()
	return &clientControl{
		observer: clientConfiguration.NewDefaultObserver(clientResolver),
		selector: clientConfiguration.NewDefaultSelector(clientResolver),
		creator:  clientConfiguration.NewDefaultCreator(clientResolver),
		manager:  clientConfiguration.NewManager(),
	}
}

func NewDefaultServerControl() (ServerControl, error) {
	if !platform.Capabilities().ServerModeSupported() {
		return nil, nil
	}

	serverResolver := serverConfiguration.NewServerResolver()
	configPath, err := serverResolver.Resolve()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve default server configuration path: %w", err)
	}
	return &serverControl{
		configPath: configPath,
		manager:    serverConfiguration.NewManager(configPath, stat.NewDefaultStat()),
	}, nil
}

func NewDefaultControls() (Controls, error) {
	server, err := NewDefaultServerControl()
	if err != nil {
		return Controls{}, err
	}
	return Controls{
		Client: NewDefaultClientControl(),
		Server: server,
	}, nil
}
