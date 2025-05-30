package server_configuration

import (
	"os"
	"path/filepath"
	"tungo/infrastructure/PAL/client_configuration"
)

type resolver struct {
}

func NewServerResolver() client_configuration.Resolver {
	return &resolver{}
}

func (r resolver) Resolve() (string, error) {
	return filepath.Join(string(os.PathSeparator), "etc", "tungo", "server_configuration.json"), nil
}
