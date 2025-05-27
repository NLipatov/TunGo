package client_configuration

import (
	"os"
	"path/filepath"
)

type DefaultResolver struct {
}

func NewDefaultResolver() Resolver {
	return DefaultResolver{}
}

func (r DefaultResolver) Resolve() (string, error) {
	return filepath.Join(string(os.PathSeparator), "etc", "tungo", "client_configuration.json"), nil
}
