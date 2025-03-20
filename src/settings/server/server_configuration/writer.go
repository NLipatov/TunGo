package server_configuration

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type writer struct {
	resolver linuxResolver
}

func newWriter(resolver linuxResolver) *writer {
	return &writer{
		resolver: resolver,
	}
}

func (w *writer) Write(data interface{}) error {
	jsonContent, jsonContentErr := json.MarshalIndent(data, "", "  ")
	if jsonContentErr != nil {
		return jsonContentErr
	}

	path, pathErr := w.resolver.resolve()
	if pathErr != nil {
		return pathErr
	}

	dir := filepath.Dir(path)
	mkdirErr := os.MkdirAll(dir, 0700)
	if mkdirErr != nil {
		return mkdirErr
	}

	file, fileErr := os.Create(path)
	if fileErr != nil {
		return fileErr
	}
	defer func(file *os.File) {
		_ = file.Close()
	}(file)

	_, writeErr := file.Write(jsonContent)
	if writeErr != nil {
		return writeErr
	}

	return nil
}
