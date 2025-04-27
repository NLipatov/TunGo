package server_configuration

import (
	"os"
	"path/filepath"
)

type linuxResolver interface {
	resolve() (string, error)
}

type resolver struct {
}

func NewServerResolver() resolver {
	return resolver{}
}

func (r resolver) resolve() (string, error) {
	return filepath.Join(string(os.PathSeparator), "etc", "tungo", "server_configuration.json"), nil
}
