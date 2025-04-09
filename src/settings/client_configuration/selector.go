package client_configuration

import (
	"fmt"
	"os"
)

// Selector is used to choose configuration as active
type Selector interface {
	Select(confPath string) error
}

type DefaultSelector struct {
	resolver Resolver
}

func NewDefaultSelector(resolver Resolver) Selector {
	return &DefaultSelector{
		resolver: resolver,
	}
}

func (s *DefaultSelector) Select(confPath string) error {
	// check if given configuration exists
	_, statErr := os.Stat(confPath)
	if statErr != nil {
		return fmt.Errorf("configuration cannot be used as file %s does not exist", confPath)
	}

	// read given configuration data
	confData, confDataErr := os.ReadFile(confPath)
	if confDataErr != nil {
		return confDataErr
	}

	// resolver resolves active configuration path
	selectedConfPath, selectedConfPathErr := s.resolver.Resolve()
	if selectedConfPathErr != nil {
		return selectedConfPathErr
	}

	// write given configuration data to active configuration path
	writeErr := os.WriteFile(selectedConfPath, confData, 0600)
	if writeErr != nil {
		return writeErr
	}

	return nil
}
