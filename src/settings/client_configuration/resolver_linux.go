package client_configuration

import (
	"os"
	"path/filepath"
)

type clientResolver struct {
}

func newClientResolver() clientResolver {
	return clientResolver{}
}

func (r clientResolver) resolve() (string, error) {
	return filepath.Join(string(os.PathSeparator), "etc", "tungo", "client_configuration.json"), nil
}
