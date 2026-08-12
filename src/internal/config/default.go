package config

import (
	"path/filepath"

	clientconfig "tungo/internal/config/client"
	serverconfig "tungo/internal/config/server"
	"tungo/internal/platform"
)

const defaultServerConfigurationPath = "/etc/tungo/server_configuration.json"

func DefaultStorageDirectory() (string, error) {
	path, err := clientconfig.NewResolver().Resolve()
	if err != nil {
		return "", err
	}
	return filepath.Dir(path), nil
}

func NewClientControl() ClientControl {
	clientResolver := clientconfig.NewResolver()
	return &clientControl{
		observer: clientconfig.NewObserver(clientResolver),
		selector: clientconfig.NewSelector(clientResolver),
		creator:  clientconfig.NewCreator(clientResolver),
		manager:  clientconfig.NewManager(),
	}
}

func NewServerControl() ServerControl {
	if !platform.ServerModeSupported() {
		return nil
	}

	return &serverControl{
		configPath: defaultServerConfigurationPath,
		manager:    serverconfig.NewManager(defaultServerConfigurationPath),
	}
}

func NewControls() Controls {
	return Controls{
		Client: NewClientControl(),
		Server: NewServerControl(),
	}
}
