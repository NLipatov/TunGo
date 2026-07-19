package server

import (
	"os"
	"path/filepath"
)

type Resolver interface {
	Resolve() (string, error)
}

type resolver struct {
}

func NewServerResolver() Resolver {
	return &resolver{}
}

func (r resolver) Resolve() (string, error) {
	return filepath.Join(string(os.PathSeparator), "etc", "tungo", "server_configuration.json"), nil
}
