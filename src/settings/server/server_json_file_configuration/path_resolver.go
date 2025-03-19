package server_json_file_configuration

import (
	"fmt"
	"os"
	"path/filepath"
)

type resolver interface {
	resolve() (string, error)
}

type pathResolver struct {
}

func newPathResolver() pathResolver {
	return pathResolver{}
}

func (r pathResolver) resolve() (string, error) {
	workingDirectory, workingDirectoryErr := os.Getwd()
	if workingDirectoryErr != nil {
		return "", fmt.Errorf("failed to resolve configuration path: %w", workingDirectoryErr)
	}

	expectedPath := filepath.Join(filepath.Dir(workingDirectory), "src", "settings", "server", "conf.json")

	return expectedPath, nil
}
