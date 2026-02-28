package server

import (
	"os"
	"path/filepath"
	"tungo/infrastructure/PAL/configuration"
)

type resolver struct {
}

func NewServerResolver() configuration.Resolver {
	return &resolver{}
}

func (r resolver) Resolve() (string, error) {
	return filepath.Join(string(os.PathSeparator), "etc", "tungo", "server_configuration.json"), nil
}
