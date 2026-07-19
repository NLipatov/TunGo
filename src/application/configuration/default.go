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
		deleter:  clientConfiguration.NewDefaultDeleter(clientResolver),
		manager:  clientConfiguration.NewManager(),
	}
}

func NewDefaultServerControl() (ServerControl, error) {
	if !platform.Capabilities().ServerModeSupported() {
		return nil, nil
	}

	serverResolver := serverConfiguration.NewServerResolver()
	serverManager, err := serverConfiguration.NewManager(serverResolver, stat.NewDefaultStat())
	if err != nil {
		return nil, fmt.Errorf("configuration error: %w", err)
	}
	return &serverControl{
		resolver: serverResolver,
		manager:  serverManager,
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
