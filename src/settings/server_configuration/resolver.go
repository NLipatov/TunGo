package server_configuration

import (
	"os"
	"path/filepath"
)

type linuxResolver interface {
	resolve() (string, error)
}

type ServerResolver struct {
}

func NewServerResolver() ServerResolver {
	return ServerResolver{}
}

func (r ServerResolver) resolve() (string, error) {
	return filepath.Join(string(os.PathSeparator), "etc", "tungo", "server_configuration.json"), nil
}
