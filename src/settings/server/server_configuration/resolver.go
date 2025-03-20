package server_configuration

import (
	"os"
	"path/filepath"
)

type linuxResolver interface {
	resolve() (string, error)
}

type Resolver struct {
}

func newResolver() Resolver {
	return Resolver{}
}

func (r Resolver) resolve() (string, error) {
	return filepath.Join(string(os.PathSeparator), "etc", "tungo", "server_configuration.json"), nil
}
