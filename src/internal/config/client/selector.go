package client

import "os"

// Selector chooses a configuration as active.
type Selector struct {
	resolver Resolver
}

func NewSelector(resolver Resolver) *Selector {
	return &Selector{
		resolver: resolver,
	}
}

func (s *Selector) Select(confPath string) error {
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
