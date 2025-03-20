package server_json_file_configuration

import (
	"encoding/json"
	"os"
)

type writer struct {
	resolver ConfigurationResolver
}

func newWriter(resolver ConfigurationResolver) *writer {
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
