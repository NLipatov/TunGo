package server_json_file_configuration

import (
	"encoding/json"
	"os"
)

type writer struct {
	path string
}

func newWriter(path string) *writer {
	return &writer{
		path: path,
	}
}

func (w *writer) Write(data interface{}) error {
	jsonContent, jsonContentErr := json.MarshalIndent(data, "", "  ")
	if jsonContentErr != nil {
		return jsonContentErr
	}

	file, fileErr := os.Create(w.path)
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
