package server_json_file_configuration

import (
	"fmt"
	"os"
	"path/filepath"
)

type ConfigurationResolver interface {
	resolve() (string, error)
}

type Resolver struct {
}

func newResolver() Resolver {
	return Resolver{}
}

func (r Resolver) resolve() (string, error) {
	workingDirectory, workingDirectoryErr := os.Getwd()
	if workingDirectoryErr != nil {
		return "", fmt.Errorf("failed to resolve configuration path: %w", workingDirectoryErr)
	}

	expectedPath := filepath.Join(filepath.Dir(workingDirectory), "src", "settings", "server", "conf.json")

	return expectedPath, nil
}
