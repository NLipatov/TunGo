package configuration

import (
	"path/filepath"

	clientConfiguration "tungo/application/configuration/internal/client"
	serverConfiguration "tungo/application/configuration/internal/server"
	"tungo/infrastructure/PAL/platform"
)

const defaultServerConfigurationPath = "/etc/tungo/server_configuration.json"

func DefaultStorageDirectory() (string, error) {
	path, err := clientConfiguration.NewResolver().Resolve()
	if err != nil {
		return "", err
	}
	return filepath.Dir(path), nil
}

func NewClientControl() ClientControl {
	clientResolver := clientConfiguration.NewResolver()
	return &clientControl{
		observer: clientConfiguration.NewObserver(clientResolver),
		selector: clientConfiguration.NewSelector(clientResolver),
		creator:  clientConfiguration.NewCreator(clientResolver),
		manager:  clientConfiguration.NewManager(),
	}
}

func NewServerControl() ServerControl {
	if !platform.Capabilities().ServerModeSupported() {
		return nil
	}

	return &serverControl{
		configPath: defaultServerConfigurationPath,
		manager:    serverConfiguration.NewManager(defaultServerConfigurationPath),
	}
}

func NewControls() Controls {
	return Controls{
		Client: NewClientControl(),
		Server: NewServerControl(),
	}
}
