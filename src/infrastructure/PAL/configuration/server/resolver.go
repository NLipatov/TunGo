package server

import (
	"os"
	"path/filepath"
	"tungo/infrastructure/PAL/configuration/client"
)

type resolver struct {
}

func NewServerResolver() client.Resolver {
	return &resolver{}
}

func (r resolver) Resolve() (string, error) {
	return filepath.Join(string(os.PathSeparator), "etc", "tungo", "server_configuration.json"), nil
}
