package server_json_file_configuration

import (
	"fmt"
	"os"
	"path/filepath"
)

type pathResolver struct {
}

func newPathResolver() pathResolver {
	return pathResolver{}
}

func (r *pathResolver) resolve() (string, error) {
	workingDirectory, workingDirectoryErr := os.Getwd()
	if workingDirectoryErr != nil {
		return "", fmt.Errorf("could not get working directory: %v", workingDirectoryErr)
	}

	expectedPath := filepath.Join(filepath.Dir(workingDirectory), "src", "settings", "server", "conf.json")
	if !r.fileExist(expectedPath) {
		return "", fmt.Errorf("configration file does not exist: %s", expectedPath)
	}

	return expectedPath, nil
}

func (r *pathResolver) fileExist(path string) bool {
	_, statErr := os.Stat(path)
	return statErr == nil
}
