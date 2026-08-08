package client

import (
	"os"
	"path/filepath"
)

type pathResolver struct{}

func NewResolver() Resolver {
	return pathResolver{}
}

func (r pathResolver) Resolve() (string, error) {
	return filepath.Join(string(os.PathSeparator), "etc", "tungo", "client_configuration.json"), nil
}
