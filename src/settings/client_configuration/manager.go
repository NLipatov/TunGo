package client_configuration

import (
	"fmt"
	"os"
)

type ClientConfigurationManager interface {
	Configuration() (*Configuration, error)
}

type Manager struct {
	resolver resolver
}

func NewManager() ClientConfigurationManager {
	return &Manager{
		resolver: newClientResolver(),
	}
}

func (m *Manager) Configuration() (*Configuration, error) {
	path, pathErr := m.resolver.resolve()
	if pathErr != nil {
		return nil, pathErr
	}

	_, statErr := os.Stat(path)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return nil, fmt.Errorf("configuration file %s does not exist", path)
		}
		return nil, statErr
	}

	return newReader(path).read()
}
