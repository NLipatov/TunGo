package configuration

import (
	"path/filepath"

	clientConfiguration "tungo/application/configuration/internal/client"
	serverConfiguration "tungo/application/configuration/internal/server"
	"tungo/infrastructure/PAL/platform"
)

const defaultServerConfigurationPath = "/etc/tungo/server_configuration.json"

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

func NewDefaultServerControl() ServerControl {
	if !platform.Capabilities().ServerModeSupported() {
		return nil
	}

	return &serverControl{
		configPath: defaultServerConfigurationPath,
		manager:    serverConfiguration.NewManager(defaultServerConfigurationPath),
	}
}

func NewDefaultControls() Controls {
	return Controls{
		Client: NewDefaultClientControl(),
		Server: NewDefaultServerControl(),
	}
}
